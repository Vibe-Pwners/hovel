package hovel

import (
	"encoding/json"
	"strings"
	"testing"
)

func validCredentialPrecondition() CredentialStampPrecondition {
	return CredentialStampPrecondition{Kind: CredentialStampPreconditionNone}
}

func validCredentialReference(projection CredentialProjection) CredentialMaterialReference {
	r := CredentialMaterialReference{Projection: projection, Form: CredentialMaterialPublic}
	switch projection {
	case CredentialProjectionBundle:
		r.BundleID = "bundle"
	case CredentialProjectionCertificateDER, CredentialProjectionPublicKeySPKI:
		r.GenerationID = "generation"
	case CredentialProjectionPrivateKeyPKCS8:
		r.Form, r.GenerationID = CredentialMaterialPrivateBytes, "generation"
	case CredentialProjectionSignerReference:
		r.Form, r.GenerationID = CredentialMaterialPrivateReference, "generation"
	case CredentialProjectionChainDER:
		r.GenerationIDs = []string{"generation"}
	case CredentialProjectionTrustDER:
		r.TrustSetGenerationID = "trust"
	case CredentialProjectionCRLDER:
		r.CRLGenerationIDs = []string{"crl"}
	}
	return r
}

func TestCredentialDeliveryEnumsAndCanonicalValues(t *testing.T) {
	for _, value := range []CredentialDeliveryCapability{CredentialDeliveryNone, CredentialDeliveryRuntime, CredentialDeliveryFiles, CredentialDeliveryStampStandard, CredentialDeliveryStampAdvanced} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialDeliveryCapability("bad").Validate(); err == nil {
		t.Fatal("invalid capability accepted")
	}
	for _, value := range []CredentialPurpose{CredentialPurposeTLSServer, CredentialPurposeTLSClient, CredentialPurposeMTLSServer, CredentialPurposeMTLSClient, CredentialPurposeDualRoleMTLS, CredentialPurposeCodeSigning, CredentialPurposeCustom} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialPurpose("bad").Validate(); err == nil {
		t.Fatal("invalid purpose accepted")
	}
	for _, value := range []CredentialConsumerType{CredentialConsumerMeshProvider, CredentialConsumerMeshListener, CredentialConsumerListeningPost, CredentialConsumerMeshNode, CredentialConsumerImplant, CredentialConsumerStager, CredentialConsumerPayload, CredentialConsumerC2Service, CredentialConsumerService, CredentialConsumerExternal} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialConsumerType("bad").Validate(); err == nil {
		t.Fatal("invalid consumer accepted")
	}
	for _, value := range []CredentialProjection{CredentialProjectionBundle, CredentialProjectionCertificateDER, CredentialProjectionPrivateKeyPKCS8, CredentialProjectionPublicKeySPKI, CredentialProjectionSignerReference, CredentialProjectionChainDER, CredentialProjectionTrustDER, CredentialProjectionCRLDER, CredentialProjectionProviderEncoding, CredentialProjectionLiteralReference} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialProjection("bad").Validate(); err == nil {
		t.Fatal("invalid projection accepted")
	}
	for _, value := range []CredentialMaterialForm{CredentialMaterialPublic, CredentialMaterialPrivateReference, CredentialMaterialPrivateBytes} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialMaterialForm("bad").Validate(); err == nil {
		t.Fatal("invalid form accepted")
	}
	for _, value := range []CredentialStampRemainderPolicy{CredentialStampRemainderPreserve, CredentialStampRemainderZeroFill, CredentialStampRemainderRequireExact} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialStampRemainderPolicy("bad").Validate(); err == nil {
		t.Fatal("invalid policy accepted")
	}
	for _, tc := range []struct {
		value CredentialCanonicalUint64
		valid bool
	}{{"0", true}, {"18446744073709551615", true}, {"", false}, {" 1", false}, {"01", false}, {"-1", false}} {
		if _, err := tc.value.Uint64(); (err == nil) != tc.valid {
			t.Fatalf("Uint64(%q) error = %v", tc.value, err)
		}
	}
}

func TestCredentialDeliveryPreconditionJSON(t *testing.T) {
	valid := []CredentialStampPrecondition{
		{Kind: CredentialStampPreconditionNone},
		{Kind: CredentialStampPreconditionBytes, Bytes: []byte{1}},
		{Kind: CredentialStampPreconditionSHA256, SHA256: strings.Repeat("a", 64), Length: "1"},
	}
	for _, value := range valid {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var got CredentialStampPrecondition
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []CredentialStampPrecondition{
		{}, {Kind: CredentialStampPreconditionNone, Bytes: []byte{1}},
		{Kind: CredentialStampPreconditionBytes}, {Kind: CredentialStampPreconditionBytes, Bytes: []byte{1}, SHA256: "x"},
		{Kind: CredentialStampPreconditionSHA256, Bytes: []byte{1}, Length: "1", SHA256: strings.Repeat("a", 64)},
		{Kind: CredentialStampPreconditionSHA256, Length: "0", SHA256: strings.Repeat("a", 64)},
		{Kind: CredentialStampPreconditionSHA256, Length: "1", SHA256: "BAD"},
	} {
		if err := value.Validate(); err == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
	var nilPrecondition *CredentialStampPrecondition
	if err := nilPrecondition.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("nil destination accepted")
	}
	for _, data := range []string{`{`, `{"kind":"bad"}`, `{"kind":"none","bytes":"AQ=="}`, `{"kind":"bytes","bytes":"AQ==","length":"1"}`} {
		var value CredentialStampPrecondition
		if err := json.Unmarshal([]byte(data), &value); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestCredentialDeliveryReferencesAndMaterials(t *testing.T) {
	projections := []CredentialProjection{CredentialProjectionBundle, CredentialProjectionCertificateDER, CredentialProjectionPrivateKeyPKCS8, CredentialProjectionPublicKeySPKI, CredentialProjectionSignerReference, CredentialProjectionChainDER, CredentialProjectionTrustDER, CredentialProjectionCRLDER}
	for _, projection := range projections {
		reference := validCredentialReference(projection)
		data, err := json.Marshal(reference)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CredentialMaterialReference
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		material := CredentialStampMaterial{Projection: projection, Credential: &reference}
		if _, err := material.Form(); err != nil {
			t.Fatal(err)
		}
		data, err = json.Marshal(material)
		if err != nil {
			t.Fatal(err)
		}
		var decodedMaterial CredentialStampMaterial
		if err := json.Unmarshal(data, &decodedMaterial); err != nil {
			t.Fatal(err)
		}
	}
	provider := CredentialStampMaterial{Projection: CredentialProjectionProviderEncoding, ProviderEncoding: &CredentialProviderEncodingMaterial{ProviderID: "provider", SchemaVersion: "v1", Form: CredentialMaterialPublic, Source: validCredentialReference(CredentialProjectionCertificateDER)}}
	literal := CredentialStampMaterial{Projection: CredentialProjectionLiteralReference, LiteralReference: &CredentialLiteralMaterialReference{Reference: "ref", SHA256: strings.Repeat("a", 64), Form: CredentialMaterialPublic}}
	for _, material := range []CredentialStampMaterial{provider, literal} {
		if _, err := material.Form(); err != nil {
			t.Fatal(err)
		}
	}
	for _, reference := range []CredentialMaterialReference{
		{}, {Projection: CredentialProjectionProviderEncoding, Form: CredentialMaterialPublic, BundleID: "x"},
		{Projection: CredentialProjectionBundle, Form: "bad", BundleID: "x"},
		{Projection: CredentialProjectionBundle, Form: CredentialMaterialPublic, BundleID: "x", GenerationID: "y"},
		{Projection: CredentialProjectionCertificateDER, Form: CredentialMaterialPrivateBytes, GenerationID: "x"},
		{Projection: CredentialProjectionPrivateKeyPKCS8, Form: CredentialMaterialPublic, GenerationID: "x"},
		{Projection: CredentialProjectionSignerReference, Form: CredentialMaterialPublic, GenerationID: "x"},
		{Projection: CredentialProjectionChainDER, Form: CredentialMaterialPublic, GenerationIDs: []string{}},
		{Projection: CredentialProjectionTrustDER, Form: CredentialMaterialPrivateBytes, TrustSetGenerationID: "x"},
		{Projection: CredentialProjectionCRLDER, Form: CredentialMaterialPublic, CRLGenerationIDs: []string{"x", "x"}},
	} {
		if err := reference.Validate(); err == nil {
			t.Fatalf("accepted %#v", reference)
		}
	}
	var nilReference *CredentialMaterialReference
	if err := nilReference.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil reference accepted")
	}
	var nilMaterial *CredentialStampMaterial
	if err := nilMaterial.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil material accepted")
	}
	for _, data := range []string{`{`, `{"projection":"bad"}`, `{"projection":"bundle","form":"public","bundleId":"x","generationId":"y"}`} {
		var value CredentialMaterialReference
		if err := json.Unmarshal([]byte(data), &value); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestCredentialDeliveryPositionAndHelpers(t *testing.T) {
	pre := validCredentialPrecondition()
	if err := validateCredentialPositionTarget("8", "", "8", "8", CredentialStampRemainderPreserve, pre, CredentialStampAddressFile); err != nil {
		t.Fatal(err)
	}
	for _, space := range []CredentialStampAddressSpace{CredentialStampAddressFile, CredentialStampAddressELFVirtual, CredentialStampAddressPERVA, CredentialStampAddressMachOVM} {
		if err := validateCredentialStampAddressSpace(space); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateCredentialStampAddressSpace("bad"); err == nil {
		t.Fatal("invalid address space accepted")
	}
	for _, args := range []struct {
		pos, base, max, align CredentialCanonicalUint64
		space                 CredentialStampAddressSpace
	}{
		{"bad", "", "8", "8", CredentialStampAddressFile}, {"8", "bad", "8", "8", CredentialStampAddressPERVA},
		{"8", "", "8", "bad", CredentialStampAddressFile},
		{"8", "", "8", "3", CredentialStampAddressFile}, {"7", "", "8", "8", CredentialStampAddressFile},
		{CredentialCanonicalUint64("18446744073709551615"), "", "8", "1", CredentialStampAddressFile},
		{"8", CredentialCanonicalUint64("18446744073709551615"), "8", "8", CredentialStampAddressPERVA},
		{"8", "16", "8", "8", CredentialStampAddressELFVirtual},
	} {
		if err := validateCredentialPositionTarget(args.pos, args.base, args.max, args.align, CredentialStampRemainderPreserve, pre, args.space); err == nil {
			t.Fatalf("accepted %#v", args)
		}
	}
	if err := validateCredentialPositionTarget("8", "8", "8", "8", CredentialStampRemainderPreserve, pre, CredentialStampAddressPERVA); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCredentialBoundedTarget("0", CredentialStampRemainderPreserve, pre); err == nil {
		t.Fatal("zero maximum accepted")
	}
	if _, err := validateCredentialBoundedTarget("8", "bad", pre); err == nil {
		t.Fatal("bad remainder accepted")
	}
	if _, err := validateCredentialBoundedTarget("1", CredentialStampRemainderPreserve, CredentialStampPrecondition{Kind: CredentialStampPreconditionBytes, Bytes: []byte{1, 2}}); err == nil {
		t.Fatal("oversize bytes accepted")
	}
	if _, err := validateCredentialBoundedTarget("1", CredentialStampRemainderPreserve, CredentialStampPrecondition{Kind: CredentialStampPreconditionSHA256, SHA256: strings.Repeat("a", 64), Length: "2"}); err == nil {
		t.Fatal("oversize hash accepted")
	}
	if _, err := validateCredentialBoundedTarget("1", CredentialStampRemainderPreserve, CredentialStampPrecondition{Kind: CredentialStampPreconditionSHA256, SHA256: strings.Repeat("a", 64), Length: "bad"}); err == nil {
		t.Fatal("invalid hash length accepted")
	}
	if err := validateCredentialReferenceList(nil, "references"); err == nil {
		t.Fatal("empty reference list accepted")
	}
	if !credentialBytesAllZero([]byte{0, 0}) || credentialBytesAllZero([]byte{0, 1}) {
		t.Fatal("zero detector")
	}
	if err := validateCredentialCanonicalText("ok", "value", 2); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", " x", "xxx", "x\n"} {
		if err := validateCredentialCanonicalText(value, "value", 2); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if err := validateCredentialSHA256(strings.Repeat("a", 64), "value"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"a", strings.Repeat("A", 64), strings.Repeat("z", 64)} {
		if err := validateCredentialSHA256(value, "value"); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestCredentialDeliveryStampTargetVariants(t *testing.T) {
	pre := validCredentialPrecondition()
	bounded := func() (CredentialCanonicalUint64, CredentialStampRemainderPolicy, CredentialStampPrecondition) {
		return "8", CredentialStampRemainderPreserve, pre
	}
	maximum, remainder, condition := bounded()
	valid := []CredentialStampTarget{
		{Kind: CredentialStampTargetNamedSlot, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
		{Kind: CredentialStampTargetFileOffset, FileOffset: &CredentialFileOffsetTarget{Offset: "8", MaximumLength: maximum, Alignment: "8", RemainderPolicy: remainder, Precondition: condition}},
		{Kind: CredentialStampTargetVirtualAddress, VirtualAddress: &CredentialVirtualAddressTarget{Address: "16", AddressSpace: CredentialStampAddressELFVirtual, ImageBase: "8", MaximumLength: maximum, Alignment: "8", RemainderPolicy: remainder, Precondition: condition}},
		{Kind: CredentialStampTargetSymbol, Symbol: &CredentialSymbolTarget{Name: "symbol", Section: ".text", MaximumLength: maximum, RemainderPolicy: remainder, Precondition: condition}},
		{Kind: CredentialStampTargetMarker, Marker: &CredentialMarkerTarget{Marker: []byte{1}, MaximumLength: maximum, RemainderPolicy: remainder, Precondition: condition}},
		{Kind: CredentialStampTargetBytePattern, BytePattern: &CredentialBytePatternTarget{Pattern: []byte{1}, Mask: []byte{0xff}, MaximumLength: maximum, RemainderPolicy: remainder, Precondition: condition}},
		{Kind: CredentialStampTargetProviderDefined, ProviderDefined: &CredentialProviderDefinedTarget{ProviderID: "provider", SchemaVersion: "v1", Value: map[string]any{"key": "value"}}},
	}
	for _, target := range valid {
		data, err := json.Marshal(target)
		if err != nil {
			t.Fatalf("marshal %#v: %v", target, err)
		}
		var decoded CredentialStampTarget
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
	}
	invalid := []CredentialStampTarget{
		{Kind: CredentialStampTargetMarker, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
		{Kind: CredentialStampTargetBytePattern, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
		{}, {Kind: CredentialStampTargetNamedSlot},
		{Kind: CredentialStampTargetNamedSlot, NamedSlot: &CredentialNamedSlotTarget{}},
		{Kind: CredentialStampTargetNamedSlot, NamedSlot: &CredentialNamedSlotTarget{Name: "x"}, FileOffset: &CredentialFileOffsetTarget{}},
		{Kind: CredentialStampTargetFileOffset},
		{Kind: CredentialStampTargetVirtualAddress},
		{Kind: CredentialStampTargetVirtualAddress, VirtualAddress: &CredentialVirtualAddressTarget{AddressSpace: "bad"}},
		{Kind: CredentialStampTargetVirtualAddress, VirtualAddress: &CredentialVirtualAddressTarget{AddressSpace: CredentialStampAddressFile}},
		{Kind: CredentialStampTargetSymbol},
		{Kind: CredentialStampTargetSymbol, Symbol: &CredentialSymbolTarget{Name: "symbol", Section: "\n", MaximumLength: maximum, RemainderPolicy: remainder, Precondition: condition}},
		{Kind: CredentialStampTargetMarker},
		{Kind: CredentialStampTargetMarker, Marker: &CredentialMarkerTarget{}},
		{Kind: CredentialStampTargetBytePattern},
		{Kind: CredentialStampTargetBytePattern, BytePattern: &CredentialBytePatternTarget{Pattern: []byte{1}, Mask: []byte{1, 2}}},
		{Kind: CredentialStampTargetBytePattern, BytePattern: &CredentialBytePatternTarget{Pattern: []byte{1}, Mask: []byte{0}}},
		{Kind: CredentialStampTargetProviderDefined},
		{Kind: CredentialStampTargetProviderDefined, ProviderDefined: &CredentialProviderDefinedTarget{SchemaVersion: "v1", Value: map[string]any{}}},
		{Kind: CredentialStampTargetProviderDefined, ProviderDefined: &CredentialProviderDefinedTarget{ProviderID: "p", Value: map[string]any{}}},
		{Kind: CredentialStampTargetProviderDefined, ProviderDefined: &CredentialProviderDefinedTarget{ProviderID: "p", SchemaVersion: "v1"}},
		{Kind: CredentialStampTargetProviderDefined, ProviderDefined: &CredentialProviderDefinedTarget{ProviderID: "p", SchemaVersion: "v1", Value: map[string]any{"bad": func() {}}}},
	}
	for _, target := range invalid {
		if err := target.Validate(); err == nil {
			t.Fatalf("accepted %#v", target)
		}
	}
	var nilTarget *CredentialStampTarget
	if err := nilTarget.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil target accepted")
	}
	for _, data := range []string{`{`, `{"kind":"bad"}`, `{"kind":"named-slot","namedSlot":{"name":"x"},"marker":{"marker":"AQ=="}}`} {
		var target CredentialStampTarget
		if err := json.Unmarshal([]byte(data), &target); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestCredentialDeliveryMaterialFailuresAndRequest(t *testing.T) {
	cert := validCredentialReference(CredentialProjectionCertificateDER)
	invalid := []CredentialStampMaterial{
		{},
		{Projection: CredentialProjectionCertificateDER},
		{Projection: CredentialProjectionCertificateDER, Credential: &CredentialMaterialReference{Projection: CredentialProjectionBundle, Form: CredentialMaterialPublic, BundleID: "b"}},
		{Projection: CredentialProjectionProviderEncoding},
		{Projection: CredentialProjectionProviderEncoding, ProviderEncoding: &CredentialProviderEncodingMaterial{SchemaVersion: "v1", Form: CredentialMaterialPublic, Source: cert}},
		{Projection: CredentialProjectionProviderEncoding, ProviderEncoding: &CredentialProviderEncodingMaterial{ProviderID: "p", Form: CredentialMaterialPublic, Source: cert}},
		{Projection: CredentialProjectionProviderEncoding, ProviderEncoding: &CredentialProviderEncodingMaterial{ProviderID: "p", SchemaVersion: "v1", Form: "bad", Source: cert}},
		{Projection: CredentialProjectionLiteralReference},
		{Projection: CredentialProjectionLiteralReference, LiteralReference: &CredentialLiteralMaterialReference{SHA256: strings.Repeat("a", 64), Form: CredentialMaterialPublic}},
		{Projection: CredentialProjectionLiteralReference, LiteralReference: &CredentialLiteralMaterialReference{Reference: "r", SHA256: strings.Repeat("a", 64), Form: "bad"}},
		{Projection: CredentialProjectionLiteralReference, LiteralReference: &CredentialLiteralMaterialReference{Reference: "r", SHA256: "bad", Form: CredentialMaterialPublic}},
	}
	for _, material := range invalid {
		if err := material.Validate(); err == nil {
			t.Fatalf("accepted %#v", material)
		}
		if _, err := material.Form(); err == nil {
			t.Fatalf("Form accepted %#v", material)
		}
	}
	for _, data := range []string{`{`, `{"projection":"bad"}`, `{"projection":"literal-reference","literalReference":{"reference":"r","sha256":"bad","form":"public"},"credential":{}}`} {
		var material CredentialStampMaterial
		if err := json.Unmarshal([]byte(data), &material); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	validMaterial := CredentialStampMaterial{Projection: CredentialProjectionCertificateDER, Credential: &cert}
	validTarget := CredentialStampTarget{Kind: CredentialStampTargetNamedSlot, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}}
	metadata := ResolvedCredentialMetadata{BundleVersion: "v1", Purpose: CredentialPurposeTLSServer, ConsumerType: CredentialConsumerService, ProfileID: "profile", CompatibilityTargetID: "target"}
	request := CredentialStampRequest{AssignmentID: "assignment", Capability: CredentialDeliveryStampStandard, SlotName: "slot", Target: validTarget, Material: validMaterial, EncodedBytes: 1, Credential: metadata}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*CredentialStampRequest){
		func(r *CredentialStampRequest) { r.AssignmentID = "" }, func(r *CredentialStampRequest) { r.Capability = "bad" },
		func(r *CredentialStampRequest) { r.SlotName = "" }, func(r *CredentialStampRequest) { r.Target = CredentialStampTarget{} },
		func(r *CredentialStampRequest) { r.Material = CredentialStampMaterial{} }, func(r *CredentialStampRequest) { r.EncodedBytes = 0 },
		func(r *CredentialStampRequest) { r.Credential.ProfileID = "" },
	}
	for _, mutate := range mutations {
		value := request
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
}

func TestCredentialDeliveryResidualHelpers(t *testing.T) {
	var precondition CredentialStampPrecondition
	if err := precondition.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("bad precondition JSON accepted")
	}
	if err := precondition.UnmarshalJSON([]byte(`{"kind":"bytes","bytes":null}`)); err == nil {
		t.Fatal("invalid decoded precondition accepted")
	}
	var target CredentialStampTarget
	if err := target.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("bad target JSON accepted")
	}
	if err := target.UnmarshalJSON([]byte(`{"kind":"named-slot","namedSlot":{"name":""}}`)); err == nil {
		t.Fatal("invalid decoded target accepted")
	}
	var reference CredentialMaterialReference
	if err := reference.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("bad reference JSON accepted")
	}
	if err := reference.UnmarshalJSON([]byte(`{"projection":"bundle","form":"public","bundleId":""}`)); err == nil {
		t.Fatal("invalid decoded reference accepted")
	}
	var material CredentialStampMaterial
	if err := material.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("bad material JSON accepted")
	}
	if err := material.UnmarshalJSON([]byte(`{"projection":"literal-reference","literalReference":{"reference":"","sha256":"bad","form":"public"}}`)); err == nil {
		t.Fatal("invalid decoded material accepted")
	}

	if err := validateCredentialProjectionForm(CredentialProjectionCertificateDER, "bad"); err == nil {
		t.Fatal("invalid form accepted")
	}
	for _, tc := range []struct {
		projection CredentialProjection
		form       CredentialMaterialForm
	}{
		{CredentialProjectionCertificateDER, CredentialMaterialPrivateBytes},
		{CredentialProjectionPrivateKeyPKCS8, CredentialMaterialPublic},
		{CredentialProjectionSignerReference, CredentialMaterialPublic},
	} {
		if err := validateCredentialProjectionForm(tc.projection, tc.form); err == nil {
			t.Fatalf("accepted %#v", tc)
		}
	}
	if err := validateCredentialCanonicalText("a\x00b", "value", 10); err == nil {
		t.Fatal("control text accepted")
	}

	pre := validCredentialPrecondition()
	if err := validateCredentialPositionTarget("8", "", "bad", "8", CredentialStampRemainderPreserve, pre, CredentialStampAddressFile); err == nil {
		t.Fatal("bad maximum accepted")
	}
	if err := validateCredentialPositionTarget("8", "", "8", "0", CredentialStampRemainderPreserve, pre, CredentialStampAddressFile); err == nil {
		t.Fatal("zero alignment accepted")
	}
	if err := validateCredentialPositionTarget("8", "", "8", "6", CredentialStampRemainderPreserve, pre, CredentialStampAddressFile); err == nil {
		t.Fatal("non-power alignment accepted")
	}
	if err := validateCredentialPositionTarget("8", "", "8", "8", "bad", pre, CredentialStampAddressFile); err == nil {
		t.Fatal("bad bounded target accepted")
	}
	if err := validateCredentialPositionTarget("8", "18446744073709551600", "8", "8", CredentialStampRemainderPreserve, pre, CredentialStampAddressPERVA); err == nil {
		t.Fatal("PE base overflow accepted")
	}
	badPre := CredentialStampPrecondition{Kind: CredentialStampPreconditionBytes}
	if _, err := validateCredentialBoundedTarget("8", CredentialStampRemainderPreserve, badPre); err == nil {
		t.Fatal("bad precondition accepted")
	}
	badHash := CredentialStampPrecondition{Kind: CredentialStampPreconditionSHA256, SHA256: strings.Repeat("a", 64), Length: "bad"}
	if _, err := validateCredentialBoundedTarget("8", CredentialStampRemainderPreserve, badHash); err == nil {
		t.Fatal("bad hash length accepted")
	}
	if !credentialBytesAllZero(nil) {
		t.Fatal("empty bytes are not all zero")
	}
	if credentialStampTargetVariantCount(CredentialStampTarget{}) != 0 || credentialMaterialReferenceVariantCount(CredentialMaterialReference{}) != 0 || credentialStampMaterialVariantCount(CredentialStampMaterial{}) != 0 {
		t.Fatal("empty variant count")
	}

	if _, err := credentialMaterialReferenceJSONField(CredentialProjectionProviderEncoding); err == nil {
		t.Fatal("provider reference field accepted")
	}
	if field, err := credentialStampMaterialJSONField(CredentialProjectionProviderEncoding); err != nil || field != "providerEncoding" {
		t.Fatalf("provider field = %q, %v", field, err)
	}
	if field, err := credentialStampMaterialJSONField(CredentialProjectionLiteralReference); err != nil || field != "literalReference" {
		t.Fatalf("literal field = %q, %v", field, err)
	}
	if err := rejectInactiveCredentialJSONFields([]byte(`{`), "", []string{"x"}, "test"); err == nil {
		t.Fatal("bad fields JSON accepted")
	}
	if err := rejectInactiveCredentialJSONFields([]byte(`{}`), "", nil, "test"); err != nil {
		t.Fatal(err)
	}
	if err := validateCredentialReferenceList([]string{" bad"}, "references"); err == nil {
		t.Fatal("bad reference accepted")
	}
}

func TestCredentialDeliveryMismatchedTaggedVariants(t *testing.T) {
	pre := validCredentialPrecondition()
	wrongTargets := []CredentialStampTarget{
		{Kind: CredentialStampTargetNamedSlot, FileOffset: &CredentialFileOffsetTarget{}},
		{Kind: CredentialStampTargetFileOffset, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
		{Kind: CredentialStampTargetVirtualAddress, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
		{Kind: CredentialStampTargetSymbol, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
		{Kind: CredentialStampTargetProviderDefined, NamedSlot: &CredentialNamedSlotTarget{Name: "slot"}},
	}
	for _, value := range wrongTargets {
		if err := value.Validate(); err == nil {
			t.Fatalf("mismatched target accepted %#v", value)
		}
	}
	for _, value := range []CredentialStampTarget{
		{Kind: CredentialStampTargetSymbol, Symbol: &CredentialSymbolTarget{Name: "", MaximumLength: "8", RemainderPolicy: CredentialStampRemainderPreserve, Precondition: pre}},
		{Kind: CredentialStampTargetSymbol, Symbol: &CredentialSymbolTarget{Name: "symbol", MaximumLength: "8", RemainderPolicy: CredentialStampRemainderPreserve, Precondition: pre}},
		{Kind: CredentialStampTargetMarker, Marker: &CredentialMarkerTarget{Marker: make([]byte, maximumCredentialStampPreconditionBytes+1)}},
		{Kind: CredentialStampTargetBytePattern, BytePattern: &CredentialBytePatternTarget{Pattern: nil, Mask: nil}},
		{Kind: CredentialStampTargetBytePattern, BytePattern: &CredentialBytePatternTarget{Pattern: []byte{1}, Mask: []byte{1, 2}}},
		{Kind: CredentialStampTargetBytePattern, BytePattern: &CredentialBytePatternTarget{Pattern: make([]byte, maximumCredentialStampPreconditionBytes+1), Mask: make([]byte, maximumCredentialStampPreconditionBytes+1)}},
	} {
		err := value.Validate()
		if value.Symbol != nil && value.Symbol.Name == "symbol" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatalf("invalid target accepted %#v", value)
		}
	}

	chain := CredentialMaterialReference{Projection: CredentialProjectionChainDER, Form: CredentialMaterialPrivateBytes, GenerationIDs: []string{"generation"}}
	crl := CredentialMaterialReference{Projection: CredentialProjectionCRLDER, Form: CredentialMaterialPrivateBytes, CRLGenerationIDs: []string{"crl"}}
	for _, value := range []CredentialMaterialReference{chain, crl} {
		if err := value.Validate(); err == nil {
			t.Fatalf("wrong form accepted %#v", value)
		}
	}
	credential := validCredentialReference(CredentialProjectionCertificateDER)
	for _, value := range []CredentialStampMaterial{
		{Projection: CredentialProjectionCertificateDER, ProviderEncoding: &CredentialProviderEncodingMaterial{}},
		{Projection: CredentialProjectionProviderEncoding, Credential: &credential},
		{Projection: CredentialProjectionLiteralReference, Credential: &credential},
	} {
		if err := value.Validate(); err == nil {
			t.Fatalf("mismatched material accepted %#v", value)
		}
	}
	metadata := validResolvedCredentialMetadata()
	for _, mutate := range []func(*ResolvedCredentialMetadata){func(m *ResolvedCredentialMetadata) { m.BundleVersion = "" }, func(m *ResolvedCredentialMetadata) { m.Purpose = "bad" }, func(m *ResolvedCredentialMetadata) { m.ConsumerType = "bad" }} {
		value := metadata
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("metadata accepted %#v", value)
		}
	}
	if err := validateCredentialProjectionForm(CredentialProjectionPrivateKeyPKCS8, CredentialMaterialPrivateBytes); err != nil {
		t.Fatal(err)
	}
}
