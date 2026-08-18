package hovel

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCredentialBundleConfiguresVerifiedTLSServer(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, root := testCredentialBundle(t, now)
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCredentialBundleJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig, err := decoded.TLSServerConfigAt(now)
	if err != nil {
		t.Fatal(err)
	}
	if serverConfig.MinVersion != tls.VersionTLS13 ||
		len(serverConfig.CurvePreferences) != 2 ||
		serverConfig.CurvePreferences[0] != tls.X25519 {
		t.Fatalf("TLS server config = %#v", serverConfig)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	server := tls.Server(serverSide, serverConfig)
	client := tls.Client(clientSide, &tls.Config{
		RootCAs:    roots,
		ServerName: "squatter.mesh.test",
		MinVersion: tls.VersionTLS13,
	})
	errors := make(chan error, 2)
	go func() { errors <- server.HandshakeContext(t.Context()) }()
	go func() { errors <- client.HandshakeContext(t.Context()) }()
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("TLS handshake: %v", err)
		}
	}
	privateAlias := decoded.PrivateKey.Data
	decoded.Clear()
	if !bytes.Equal(privateAlias, make([]byte, len(privateAlias))) {
		t.Fatal("CredentialBundle.Clear() did not clear private-key bytes")
	}
}

func TestCredentialBundleDecodeFailsClosed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, _ := testCredentialBundle(t, now)
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	certificate := wire["certificate"].(map[string]any)
	certificate["unknown"] = true
	encoded, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCredentialBundleJSON(encoded); err == nil {
		t.Fatal("DecodeCredentialBundleJSON() accepted an unknown nested field")
	}

	badKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	badPKCS8, err := x509.MarshalPKCS8PrivateKey(badKey)
	if err != nil {
		t.Fatal(err)
	}
	bundle.PrivateKey.Data = badPKCS8
	encoded, err = json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCredentialBundleJSON(encoded); err == nil {
		t.Fatal("DecodeCredentialBundleJSON() accepted a mismatched private key")
	}
}

func TestCredentialBundleValidateAtEnforcesPurposeAndTrust(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, _ := testCredentialBundle(t, now)
	bundle.Purpose = CredentialPurposeDualRoleMTLS
	if err := bundle.ValidateAt(now); err == nil {
		t.Fatal("CredentialBundle.ValidateAt() accepted a dual-role certificate without client authentication usage")
	}

	bundle, _ = testCredentialBundle(t, now)
	bundle.TrustAnchors = []CredentialBundleCertificate{{
		GenerationID: "generation-untrusted-leaf",
		CredentialBundleBinary: CredentialBundleBinary{
			MediaType: CredentialBundleMediaCertificate,
			Encoding:  CredentialBundleEncodingBase64DER,
			Data:      append(CredentialBytes(nil), bundle.Certificate.Data...),
		},
	}}
	if err := bundle.ValidateAt(now); err == nil {
		t.Fatal("CredentialBundle.ValidateAt() accepted a non-self-signed trust anchor")
	}
}

func TestCredentialBundleWireAndHelperFailures(t *testing.T) {
	if _, err := DecodeCredentialBundleJSON(nil); err == nil {
		t.Fatal("empty JSON accepted")
	}
	if _, err := DecodeCredentialBundleJSON([]byte(`{}`)); err == nil {
		t.Fatal("invalid bundle accepted")
	}
	if _, err := DecodeCredentialBundleJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	var nilBundle *CredentialBundle
	nilBundle.Clear()
	if got := fmt.Sprintf("%s %#v", CredentialBundle{}, CredentialBundle{}); got != "<credential bundle redacted> <credential bundle redacted>" {
		t.Fatalf("redaction = %q", got)
	}

	validBinary := CredentialBundleBinary{MediaType: CredentialBundleMediaCertificate, Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}}
	for _, tc := range []struct {
		binary CredentialBundleBinary
		media  string
		valid  bool
	}{
		{validBinary, CredentialBundleMediaCertificate, true},
		{CredentialBundleBinary{MediaType: CredentialBundleMediaPublicKey, Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}}, CredentialBundleMediaPublicKey, true},
		{CredentialBundleBinary{MediaType: CredentialBundleMediaPrivateKey, Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}}, CredentialBundleMediaPrivateKey, true},
		{CredentialBundleBinary{MediaType: CredentialBundleMediaCRL, Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}}, CredentialBundleMediaCRL, true},
		{CredentialBundleBinary{}, CredentialBundleMediaCertificate, false},
		{CredentialBundleBinary{MediaType: "bad", Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}}, CredentialBundleMediaCertificate, false},
		{CredentialBundleBinary{MediaType: CredentialBundleMediaCertificate, Encoding: "bad", Data: []byte{1}}, CredentialBundleMediaCertificate, false},
	} {
		_, err := validateCredentialBundleBinary(tc.binary, tc.media, "test")
		if (err == nil) != tc.valid {
			t.Fatalf("binary %#v error = %v", tc, err)
		}
	}
	if _, err := parseCredentialBundleCertificate([]byte{1}, "test"); err == nil {
		t.Fatal("bad certificate accepted")
	}
	if err := validateCredentialBundleFingerprint("bad", []byte{1}, "test"); err == nil {
		t.Fatal("bad digest accepted")
	}
	if err := validateCredentialBundleFingerprint(strings.Repeat("A", 64), []byte{1}, "test"); err == nil {
		t.Fatal("uppercase digest accepted")
	}
	if err := validateCredentialBundleFingerprint(strings.Repeat("z", 64), []byte{1}, "test"); err == nil {
		t.Fatal("nonhex digest accepted")
	}
	if err := validateCredentialBundleFingerprint(strings.Repeat("0", 64), []byte{1}, "test"); err == nil {
		t.Fatal("wrong digest accepted")
	}
}

func TestCredentialBundleKeyEstablishmentContract(t *testing.T) {
	valid := []struct {
		policy string
		groups []string
	}{
		{CredentialKeyEstablishmentNotApplicable, nil},
		{CredentialKeyEstablishmentClassicalCompatible, []string{"x25519"}},
		{CredentialKeyEstablishmentHybridPQPreferred, []string{"x25519", "x25519-mlkem768"}},
		{CredentialKeyEstablishmentHybridPQRequired, []string{"x25519-mlkem768"}},
	}
	for _, tc := range valid {
		if err := validateCredentialKeyEstablishment(tc.policy, tc.groups); err != nil {
			t.Fatalf("%#v: %v", tc, err)
		}
	}
	invalid := []struct {
		policy string
		groups []string
	}{
		{"bad", nil}, {CredentialKeyEstablishmentNotApplicable, []string{"x25519"}},
		{CredentialKeyEstablishmentClassicalCompatible, nil}, {CredentialKeyEstablishmentClassicalCompatible, []string{"x25519-mlkem768"}},
		{CredentialKeyEstablishmentHybridPQPreferred, []string{"x25519"}}, {CredentialKeyEstablishmentHybridPQPreferred, []string{"x25519-mlkem768"}},
		{CredentialKeyEstablishmentHybridPQRequired, nil}, {CredentialKeyEstablishmentHybridPQRequired, []string{"x25519"}},
		{CredentialKeyEstablishmentClassicalCompatible, []string{"bad"}}, {CredentialKeyEstablishmentClassicalCompatible, []string{"x25519", "x25519"}},
	}
	for _, tc := range invalid {
		if err := validateCredentialKeyEstablishment(tc.policy, tc.groups); err == nil {
			t.Fatalf("accepted %#v", tc)
		}
	}
	groups := []string{"x25519-mlkem768", "secp256r1-mlkem768", "secp384r1-mlkem1024", "x25519", "secp256r1", "secp384r1", "secp521r1"}
	if curves, err := credentialBundleTLSCurves(groups); err != nil || len(curves) != len(groups) {
		t.Fatalf("curves = %#v, %v", curves, err)
	}
	if _, err := credentialBundleTLSCurves([]string{"bad"}); err == nil {
		t.Fatal("bad group accepted")
	}
}

func TestCredentialBundleValidationMutations(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mutations := []func(*CredentialBundle){
		func(b *CredentialBundle) { b.SchemaVersion = "bad" }, func(b *CredentialBundle) { b.ID = "" },
		func(b *CredentialBundle) { b.AssignmentID = " bad" }, func(b *CredentialBundle) { b.Generation = 0 },
		func(b *CredentialBundle) { b.Purpose = "bad" }, func(b *CredentialBundle) { b.KeyEstablishmentPolicy = "bad" },
		func(b *CredentialBundle) { b.Certificate.Data = []byte{1} }, func(b *CredentialBundle) { b.PublicKey.Data = []byte{1} },
		func(b *CredentialBundle) { b.PublicKey.Data = append(CredentialBytes(nil), b.Certificate.Data...) },
		func(b *CredentialBundle) { b.Fingerprints.CertificateSHA256 = "bad" }, func(b *CredentialBundle) { b.Fingerprints.PublicKeySHA256 = strings.Repeat("0", 64) },
		func(b *CredentialBundle) {
			b.PrivateKeyRef = &CredentialBundleKeyReference{KeyID: "key", ProviderID: "provider"}
		},
		func(b *CredentialBundle) { b.PrivateKey.Data = []byte{1} },
		func(b *CredentialBundle) { b.NotBefore = time.Time{} }, func(b *CredentialBundle) { b.NotAfter = b.NotBefore },
		func(b *CredentialBundle) { b.NotAfter = b.NotAfter.Add(time.Second) },
	}
	for index, mutate := range mutations {
		bundle, _ := testCredentialBundle(t, now)
		mutate(&bundle)
		if err := bundle.Validate(); err == nil {
			t.Fatalf("mutation %d accepted", index)
		}
	}
	bundle, _ := testCredentialBundle(t, now)
	bundle.PrivateKey = nil
	bundle.PrivateKeyRef = &CredentialBundleKeyReference{KeyID: "key", ProviderID: "provider", Capabilities: []string{"sign"}}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialBundleKeyReference){
		func(r *CredentialBundleKeyReference) { r.KeyID = "" }, func(r *CredentialBundleKeyReference) { r.ProviderID = "" },
		func(r *CredentialBundleKeyReference) { r.Capabilities = []string{"x", "x"} }, func(r *CredentialBundleKeyReference) { r.Capabilities = []string{"\n"} },
	} {
		candidate := bundle
		ref := *bundle.PrivateKeyRef
		candidate.PrivateKeyRef = &ref
		mutate(&ref)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("reference mutation accepted: %#v", ref)
		}
	}
	if err := bundle.ValidateAt(time.Time{}); err == nil {
		t.Fatal("zero verification time accepted")
	}
	if err := bundle.ValidateAt(now.Add(-2 * time.Hour)); err == nil {
		t.Fatal("premature bundle accepted")
	}
	if _, err := bundle.TLSServerConfigAt(now); err == nil {
		t.Fatal("reference-only server key accepted")
	}
}

func testCredentialBundle(
	t *testing.T,
	now time.Time,
) (CredentialBundle, *x509.Certificate) {
	bundle, root, _ := testCredentialBundleWithRootKey(t, now)
	return bundle, root
}

func testCredentialBundleWithRootKey(
	t *testing.T,
	now time.Time,
) (CredentialBundle, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Hovel test root"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	rootDER, err := x509.CreateCertificate(
		rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "squatter.mesh.test"},
		DNSNames:     []string{"squatter.mesh.test"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(
		rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	certificateDigest := sha256.Sum256(leafDER)
	publicDigest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return CredentialBundle{
		SchemaVersion:           CredentialBundleSchemaV1,
		ID:                      "bundle-test",
		AssignmentID:            "assignment-test",
		CertificateID:           "certificate-test",
		CertificateGenerationID: "generation-leaf",
		Generation:              1,
		Purpose:                 CredentialPurposeTLSServer,
		CompatibilityTargetID:   "portable-x509",
		CompatibilityVersion:    "1",
		KeyEstablishmentPolicy:  CredentialKeyEstablishmentClassicalCompatible,
		TLSNamedGroups:          []string{"x25519", "secp256r1"},
		Certificate: CredentialBundleBinary{
			MediaType: CredentialBundleMediaCertificate,
			Encoding:  CredentialBundleEncodingBase64DER,
			Data:      leafDER,
		},
		PublicKey: CredentialBundleBinary{
			MediaType: CredentialBundleMediaPublicKey,
			Encoding:  CredentialBundleEncodingBase64DER,
			Data:      leaf.RawSubjectPublicKeyInfo,
		},
		PrivateKey: &CredentialBundleBinary{
			MediaType: CredentialBundleMediaPrivateKey,
			Encoding:  CredentialBundleEncodingBase64DER,
			Data:      privateKey,
		},
		TrustAnchors: []CredentialBundleCertificate{{
			GenerationID: "generation-root",
			CredentialBundleBinary: CredentialBundleBinary{
				MediaType: CredentialBundleMediaCertificate,
				Encoding:  CredentialBundleEncodingBase64DER,
				Data:      rootDER,
			},
		}},
		Fingerprints: CredentialBundleFingerprints{
			CertificateSHA256: hex.EncodeToString(certificateDigest[:]),
			PublicKeySHA256:   hex.EncodeToString(publicDigest[:]),
		},
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
	}, root, rootKey
}
