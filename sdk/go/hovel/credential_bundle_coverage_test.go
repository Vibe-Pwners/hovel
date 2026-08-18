package hovel

import (
	"crypto"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestCredentialBundleCollectionValidation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mutations := []func(*CredentialBundle){
		func(b *CredentialBundle) { b.Certificate.MediaType = "bad" },
		func(b *CredentialBundle) { b.PublicKey.MediaType = "bad" },
		func(b *CredentialBundle) {
			root, err := x509.ParseCertificate(b.TrustAnchors[0].Data)
			if err != nil {
				t.Fatal(err)
			}
			b.PublicKey.Data = append(CredentialBytes(nil), root.RawSubjectPublicKeyInfo...)
		},
		func(b *CredentialBundle) { b.PrivateKey.Encoding = "bad" },
		func(b *CredentialBundle) {
			key, err := ecdh.X25519().GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			data, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			b.PrivateKey.Data = data
		},
		func(b *CredentialBundle) {
			b.Chain = make([]CredentialBundleCertificate, credentialBundleMaximumCertificateCount+1)
		},
		func(b *CredentialBundle) {
			b.CertificateRevocationLists = make([]CredentialBundleCRL, credentialBundleMaximumCRLCount+1)
		},
		func(b *CredentialBundle) {
			b.PrivateKey = nil
			b.PrivateKeyRef = &CredentialBundleKeyReference{KeyID: "key", ProviderID: "provider", Capabilities: make([]string, maximumCredentialReferenceCapabilities+1)}
		},
		func(b *CredentialBundle) {
			b.TrustAnchors[0].GenerationID = b.CertificateGenerationID
		},
		func(b *CredentialBundle) { b.TrustAnchors[0].GenerationID = "" },
		func(b *CredentialBundle) { b.TrustAnchors[0].Encoding = "bad" },
		func(b *CredentialBundle) { b.TrustAnchors[0].Data = []byte{1} },
		func(b *CredentialBundle) {
			member := b.TrustAnchors[0]
			b.Chain = []CredentialBundleCertificate{member}
		},
		func(b *CredentialBundle) {
			b.CertificateRevocationLists = []CredentialBundleCRL{{GenerationID: ""}}
		},
		func(b *CredentialBundle) {
			b.CertificateRevocationLists = []CredentialBundleCRL{{GenerationID: "crl", IssuerGenerationID: ""}}
		},
		func(b *CredentialBundle) {
			b.CertificateRevocationLists = []CredentialBundleCRL{
				{GenerationID: "crl", IssuerGenerationID: "issuer"},
				{GenerationID: "crl", IssuerGenerationID: "issuer"},
			}
		},
		func(b *CredentialBundle) {
			b.CertificateRevocationLists = []CredentialBundleCRL{{
				GenerationID: "crl", IssuerGenerationID: "issuer",
				CredentialBundleBinary: CredentialBundleBinary{MediaType: CredentialBundleMediaCRL, Encoding: "bad", Data: []byte{1}},
			}}
		},
		func(b *CredentialBundle) {
			b.CertificateRevocationLists = []CredentialBundleCRL{{
				GenerationID: "crl", IssuerGenerationID: "issuer",
				CredentialBundleBinary: CredentialBundleBinary{MediaType: CredentialBundleMediaCRL, Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}},
			}}
		},
	}
	for index, mutate := range mutations {
		bundle, _ := testCredentialBundle(t, now)
		mutate(&bundle)
		if err := bundle.Validate(); err == nil {
			t.Fatalf("collection mutation %d accepted", index)
		}
	}
	withoutAssignment, _ := testCredentialBundle(t, now)
	withoutAssignment.AssignmentID = ""
	if err := withoutAssignment.Validate(); err != nil {
		t.Fatalf("optional assignment rejected: %v", err)
	}
	referenceOnly, _ := testCredentialBundle(t, now)
	referenceOnly.PrivateKey = nil
	referenceOnly.PrivateKeyRef = &CredentialBundleKeyReference{KeyID: "key", ProviderID: "provider"}
	if err := referenceOnly.Validate(); err != nil {
		t.Fatalf("reference without capabilities rejected: %v", err)
	}
	badNotBefore, _ := testCredentialBundle(t, now)
	badNotBefore.NotBefore = badNotBefore.NotBefore.Add(time.Second)
	if err := badNotBefore.Validate(); err == nil {
		t.Fatal("mismatched not-before accepted")
	}
}

func credentialBundleTestCRL(t *testing.T, issuer *x509.Certificate, key crypto.Signer, now time.Time, revoked bool) []byte {
	t.Helper()
	template := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: now.Add(-time.Minute),
		NextUpdate: now.Add(time.Hour),
	}
	if revoked {
		template.RevokedCertificateEntries = []x509.RevocationListEntry{{
			SerialNumber:   big.NewInt(2),
			RevocationTime: now.Add(-time.Second),
		}}
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, issuer, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestCredentialBundleDirectTrustChainFailures(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, _ := testCredentialBundle(t, now)
	leaf, err := x509.ParseCertificate(bundle.Certificate.Data)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(bundle.TrustAnchors[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCredentialBundleChainAt(leaf, nil, nil, now, CredentialPurposeTLSServer); err == nil {
		t.Fatal("non-self-signed leaf accepted without trust")
	}
	if err := verifyCredentialBundleChainAt(leaf, []*x509.Certificate{leaf}, nil, now, CredentialPurposeTLSServer); err == nil {
		t.Fatal("mismatched chain accepted")
	}
	badParent := *root
	badParent.RawSubject = append([]byte(nil), leaf.RawIssuer...)
	badParent.PublicKey = leaf.PublicKey
	if err := verifyCredentialBundleChainAt(leaf, []*x509.Certificate{&badParent}, nil, now, CredentialPurposeTLSServer); err == nil {
		t.Fatal("invalid chain signature accepted")
	}
	if err := verifyCredentialBundleChainAt(leaf, []*x509.Certificate{root}, nil, now, CredentialPurposeTLSServer); err == nil {
		t.Fatal("nonempty chain accepted without explicit trust")
	}
	if err := verifyCredentialBundleChainAt(root, nil, nil, now, CredentialPurposeCustom); err != nil {
		t.Fatalf("self-signed root rejected: %v", err)
	}
	badSelf := *root
	badSelf.PublicKey = leaf.PublicKey
	if err := verifyCredentialBundleChainAt(&badSelf, nil, nil, now, CredentialPurposeCustom); err == nil {
		t.Fatal("invalid self-signature accepted")
	}
	if err := verifyCredentialBundleChainAt(root, nil, []*x509.Certificate{root}, now, CredentialPurposeCustom); err != nil {
		t.Fatalf("identical explicit trust rejected: %v", err)
	}
	_, unrelatedRoot := testCredentialBundle(t, now)
	unrelatedRoot.RawSubject = []byte("unrelated")
	unrelatedRoot.RawIssuer = []byte("unrelated")
	if err := verifyCredentialBundleChainAt(leaf, nil, []*x509.Certificate{unrelatedRoot}, now, CredentialPurposeTLSServer); err == nil {
		t.Fatal("unrelated trust root accepted")
	}
	if err := verifyCredentialBundleChainAt(leaf, nil, []*x509.Certificate{root}, now, CredentialPurposeTLSServer); err != nil {
		t.Fatalf("direct trust rejected: %v", err)
	}
	if err := verifyCredentialBundleChainAt(leaf, nil, []*x509.Certificate{leaf}, now, CredentialPurposeTLSServer); err == nil {
		t.Fatal("non-self-signed trust anchor accepted")
	}
}

func selfSignedCredentialBundle(t *testing.T, now time.Time, purpose CredentialPurpose) CredentialBundle {
	t.Helper()
	bundle, root, rootKey := testCredentialBundleWithRootKey(t, now)
	if purpose == CredentialPurposeMTLSServer {
		template := *root
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		der, err := x509.CreateCertificate(rand.Reader, &template, &template, &rootKey.PublicKey, rootKey)
		if err != nil {
			t.Fatal(err)
		}
		root, err = x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	bundle.CertificateGenerationID = "generation-root"
	bundle.Purpose = purpose
	bundle.Certificate.Data = append(CredentialBytes(nil), root.Raw...)
	bundle.PublicKey.Data = append(CredentialBytes(nil), root.RawSubjectPublicKeyInfo...)
	bundle.PrivateKey.Data = privateKey
	bundle.Chain = nil
	bundle.TrustAnchors = nil
	bundle.NotBefore, bundle.NotAfter = root.NotBefore, root.NotAfter
	certificateDigest := sha256.Sum256(bundle.Certificate.Data)
	publicDigest := sha256.Sum256(bundle.PublicKey.Data)
	bundle.Fingerprints.CertificateSHA256 = hex.EncodeToString(certificateDigest[:])
	bundle.Fingerprints.PublicKeySHA256 = hex.EncodeToString(publicDigest[:])
	return bundle
}

func TestCredentialBundleCRLVerificationBranches(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, root, rootKey := testCredentialBundleWithRootKey(t, now)
	makeMember := func(der []byte, issuer string) CredentialBundleCRL {
		return CredentialBundleCRL{
			GenerationID: "crl-generation", IssuerGenerationID: issuer,
			CredentialBundleBinary: CredentialBundleBinary{
				MediaType: CredentialBundleMediaCRL,
				Encoding:  CredentialBundleEncodingBase64DER,
				Data:      der,
			},
		}
	}
	bundle.CertificateRevocationLists = []CredentialBundleCRL{
		makeMember(credentialBundleTestCRL(t, root, rootKey, now, false), "generation-root"),
	}
	if err := bundle.ValidateAt(now); err != nil {
		t.Fatalf("fresh CRL rejected: %v", err)
	}
	stale := bundle
	staleList := &x509.RevocationList{
		Number: big.NewInt(2), ThisUpdate: now.Add(-time.Hour), NextUpdate: now,
	}
	staleDER, err := x509.CreateRevocationList(rand.Reader, staleList, root, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	stale.CertificateRevocationLists = []CredentialBundleCRL{makeMember(staleDER, "generation-root")}
	if err := stale.ValidateAt(now); err == nil {
		t.Fatal("stale CRL accepted")
	}

	missing := bundle
	missing.CertificateRevocationLists = append([]CredentialBundleCRL(nil), bundle.CertificateRevocationLists...)
	missing.CertificateRevocationLists[0].IssuerGenerationID = "missing"
	if err := missing.ValidateAt(now); err == nil {
		t.Fatal("missing CRL issuer accepted")
	}

	mismatched := bundle
	mismatched.CertificateRevocationLists = append([]CredentialBundleCRL(nil), bundle.CertificateRevocationLists...)
	mismatched.CertificateRevocationLists[0].IssuerGenerationID = bundle.CertificateGenerationID
	if err := mismatched.ValidateAt(now); err == nil {
		t.Fatal("mismatched CRL issuer accepted")
	}

	revoked := bundle
	revoked.CertificateRevocationLists = []CredentialBundleCRL{
		makeMember(credentialBundleTestCRL(t, root, rootKey, now, true), "generation-root"),
	}
	if err := revoked.ValidateAt(now); err == nil {
		t.Fatal("revoked leaf accepted")
	}

	duplicate := bundle
	duplicate.CertificateRevocationLists = append([]CredentialBundleCRL(nil), bundle.CertificateRevocationLists...)
	duplicate.CertificateRevocationLists = append(duplicate.CertificateRevocationLists, duplicate.CertificateRevocationLists[0])
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate valid CRL generation accepted")
	}

	badSignature := bundle
	badSignature.CertificateRevocationLists = append([]CredentialBundleCRL(nil), bundle.CertificateRevocationLists...)
	badSignature.CertificateRevocationLists[0].Data = append(CredentialBytes(nil), bundle.CertificateRevocationLists[0].Data...)
	badSignature.CertificateRevocationLists[0].Data[len(badSignature.CertificateRevocationLists[0].Data)-1] ^= 1
	if err := badSignature.ValidateAt(now); err == nil {
		t.Fatal("bad CRL signature accepted")
	}

	nonmatchingRevocation := bundle
	member := makeMember(credentialBundleTestCRL(t, root, rootKey, now, false), "generation-root")
	list, err := x509.ParseRevocationList(member.Data)
	if err != nil {
		t.Fatal(err)
	}
	list.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(999), RevocationTime: now.Add(-time.Second)}}
	member.Data, err = x509.CreateRevocationList(rand.Reader, list, root, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	nonmatchingRevocation.CertificateRevocationLists = []CredentialBundleCRL{member}
	if err := nonmatchingRevocation.ValidateAt(now); err != nil {
		t.Fatalf("unrelated revocation rejected bundle: %v", err)
	}
}

func TestCredentialBundleCRLFreshnessConditions(t *testing.T) {
	now := time.Now().UTC()
	for index, list := range []*x509.RevocationList{
		{},
		{ThisUpdate: now.Add(-time.Minute)},
		{ThisUpdate: now, NextUpdate: now},
		{ThisUpdate: now.Add(time.Minute), NextUpdate: now.Add(time.Hour)},
		{ThisUpdate: now.Add(-time.Hour), NextUpdate: now},
	} {
		if credentialBundleCRLIsFresh(list, now) {
			t.Fatalf("stale CRL condition %d reported fresh", index)
		}
	}
	if !credentialBundleCRLIsFresh(&x509.RevocationList{
		ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
	}, now) {
		t.Fatal("fresh CRL reported stale")
	}
}

func TestCredentialBundleChainAndTLSConfigurationBranches(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, _ := testCredentialBundle(t, now)
	rootMember := bundle.TrustAnchors[0]
	rootMember.GenerationID = "generation-chain-root"
	bundle.Chain = []CredentialBundleCertificate{rootMember}
	if err := bundle.ValidateAt(now); err != nil {
		t.Fatalf("explicit chain rejected: %v", err)
	}
	config, err := bundle.TLSServerConfigAt(now)
	if err != nil || len(config.Certificates[0].Certificate) != 2 {
		t.Fatalf("TLS chain config = %#v, %v", config, err)
	}
	mtls := bundle
	mtls.Purpose = CredentialPurposeMTLSServer
	mtlsConfig, err := mtls.TLSServerConfigAt(now)
	if err != nil || mtlsConfig.ClientAuth != tls.RequireAndVerifyClientCert || mtlsConfig.ClientCAs == nil {
		t.Fatalf("mTLS config = %#v, %v", mtlsConfig, err)
	}

	invalid := bundle
	invalid.SchemaVersion = "bad"
	if err := invalid.ValidateAt(now); err == nil {
		t.Fatal("ValidateAt accepted structurally invalid bundle")
	}
	if _, err := invalid.TLSServerConfigAt(now); err == nil {
		t.Fatal("TLS config accepted structurally invalid bundle")
	}

	client := bundle
	client.Purpose = CredentialPurposeTLSClient
	if _, err := client.TLSServerConfigAt(now); err == nil {
		t.Fatal("client credential configured a TLS server")
	}
	custom := selfSignedCredentialBundle(t, now, CredentialPurposeCustom)
	if _, err := custom.TLSServerConfigAt(now); err == nil {
		t.Fatal("custom-purpose credential configured a TLS server")
	}
	selfSignedMTLS := selfSignedCredentialBundle(t, now, CredentialPurposeMTLSServer)
	if _, err := selfSignedMTLS.TLSServerConfigAt(now); err != nil {
		t.Fatalf("self-signed mTLS credential rejected: %v", err)
	}

	badKey := bundle
	badKey.PrivateKey = &CredentialBundleBinary{
		MediaType: CredentialBundleMediaPrivateKey,
		Encoding:  CredentialBundleEncodingBase64DER,
		Data:      []byte{1},
	}
	if _, err := badKey.TLSServerConfigAt(now); err == nil {
		t.Fatal("malformed server key configured TLS")
	}

	badGroups := bundle
	badGroups.TLSNamedGroups = []string{"bad"}
	if _, err := badGroups.TLSServerConfigAt(now); err == nil {
		t.Fatal("unsupported TLS group configured TLS")
	}
}

func TestCredentialBundleResidualHelpers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, root := testCredentialBundle(t, now)
	if _, err := parseCredentialBundleCertificate(append(append([]byte(nil), root.Raw...), 0), "trailing"); err == nil {
		t.Fatal("certificate trailing data accepted")
	}
	if curves, err := credentialBundleTLSCurves(nil); err != nil || len(curves) != 0 {
		t.Fatalf("empty curves = %#v, %v", curves, err)
	}
	certificate := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning, x509.ExtKeyUsageServerAuth}}
	if err := verifyCredentialBundlePurpose(certificate, CredentialPurposeCodeSigning); err != nil {
		t.Fatal(err)
	}
	badRoots := x509.NewCertPool()
	if err := verifyCredentialBundlePKIX(root, nil, badRoots, now, CredentialPurposeCustom); err == nil {
		t.Fatal("PKIX accepted missing root")
	}
	if err := validateCredentialBundleAggregateSize(CredentialBundle{
		Certificate: CredentialBundleBinary{Data: make(CredentialBytes, credentialBundleMaximumBinaryBytes)},
		PrivateKey:  &CredentialBundleBinary{Data: CredentialBytes{1}},
	}); err == nil {
		t.Fatal("private-key aggregate overflow accepted")
	}
	if err := validateCredentialBundleAggregateSize(CredentialBundle{
		Certificate: CredentialBundleBinary{Data: make(CredentialBytes, credentialBundleMaximumBinaryBytes)},
		Chain:       []CredentialBundleCertificate{{CredentialBundleBinary: CredentialBundleBinary{Data: CredentialBytes{1}}}},
	}); err == nil {
		t.Fatal("chain aggregate overflow accepted")
	}
	if err := validateCredentialBundleAggregateSize(CredentialBundle{
		Certificate:                CredentialBundleBinary{Data: make(CredentialBytes, credentialBundleMaximumBinaryBytes)},
		CertificateRevocationLists: []CredentialBundleCRL{{CredentialBundleBinary: CredentialBundleBinary{Data: CredentialBytes{1}}}},
	}); err == nil {
		t.Fatal("CRL aggregate overflow accepted")
	}
	_ = bundle
}

func TestCredentialBundleClearCoversEveryOwnedCollection(t *testing.T) {
	bundle := CredentialBundle{
		Certificate:                CredentialBundleBinary{Data: CredentialBytes{1}},
		PublicKey:                  CredentialBundleBinary{Data: CredentialBytes{2}},
		PrivateKey:                 &CredentialBundleBinary{Data: CredentialBytes{3}},
		Chain:                      []CredentialBundleCertificate{{CredentialBundleBinary: CredentialBundleBinary{Data: CredentialBytes{4}}}},
		TrustAnchors:               []CredentialBundleCertificate{{CredentialBundleBinary: CredentialBundleBinary{Data: CredentialBytes{5}}}},
		CertificateRevocationLists: []CredentialBundleCRL{{CredentialBundleBinary: CredentialBundleBinary{Data: CredentialBytes{6}}}},
	}
	aliases := [][]byte{bundle.Certificate.Data, bundle.PublicKey.Data, bundle.PrivateKey.Data, bundle.Chain[0].Data, bundle.TrustAnchors[0].Data, bundle.CertificateRevocationLists[0].Data}
	bundle.Clear()
	for index, alias := range aliases {
		if alias[0] != 0 {
			t.Fatalf("owned byte collection %d was not cleared", index)
		}
	}
}

func TestCredentialBundlePurposeAndUsageMatrix(t *testing.T) {
	certificate := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{
		x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth,
		x509.ExtKeyUsageCodeSigning,
	}}
	for _, purpose := range []CredentialPurpose{
		CredentialPurposeTLSServer, CredentialPurposeMTLSServer,
		CredentialPurposeTLSClient, CredentialPurposeMTLSClient,
		CredentialPurposeDualRoleMTLS, CredentialPurposeCodeSigning,
		CredentialPurposeCustom,
	} {
		if err := verifyCredentialBundlePurpose(certificate, purpose); err != nil {
			t.Fatalf("purpose %q: %v", purpose, err)
		}
		if len(credentialBundleKeyUsages(purpose)) == 0 {
			t.Fatalf("purpose %q has no verification usages", purpose)
		}
	}
	for _, purpose := range []CredentialPurpose{CredentialPurposeTLSServer, CredentialPurposeTLSClient, CredentialPurposeCodeSigning} {
		if err := verifyCredentialBundlePurpose(&x509.Certificate{}, purpose); err == nil {
			t.Fatalf("purpose %q accepted without its EKU", purpose)
		}
	}
}

func TestCredentialBundleBinaryBoundsAndAggregateBounds(t *testing.T) {
	tooLarge := CredentialBundleBinary{
		MediaType: CredentialBundleMediaCertificate,
		Encoding:  CredentialBundleEncodingBase64DER,
		Data:      make(CredentialBytes, credentialBundleMaximumCertificateBytes+1),
	}
	if _, err := validateCredentialBundleBinary(tooLarge, CredentialBundleMediaCertificate, "certificate"); err == nil {
		t.Fatal("oversized certificate accepted")
	}
	unknown := CredentialBundleBinary{MediaType: "custom", Encoding: CredentialBundleEncodingBase64DER, Data: []byte{1}}
	if _, err := validateCredentialBundleBinary(unknown, "custom", "custom"); err != nil {
		t.Fatal(err)
	}
	if err := validateCredentialBundleAggregateSize(CredentialBundle{
		Certificate: CredentialBundleBinary{Data: make(CredentialBytes, credentialBundleMaximumBinaryBytes)},
		PublicKey:   CredentialBundleBinary{Data: CredentialBytes{1}},
	}); err == nil {
		t.Fatal("aggregate binary overflow accepted")
	}
	if err := validateCredentialBundleFingerprint(strings.Repeat("0", 64), nil, "empty"); err == nil {
		t.Fatal("incorrect empty fingerprint accepted")
	}
}
