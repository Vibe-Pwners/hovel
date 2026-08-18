package hovel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func credentialDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func validCredentialProviderTarget() CredentialProviderTarget {
	return CredentialProviderTarget{ModuleID: "module", ProviderID: "provider", ProviderVersion: "v1", DescriptorSHA256: strings.Repeat("a", 64)}
}

func validResolvedCredentialMetadata() ResolvedCredentialMetadata {
	return ResolvedCredentialMetadata{BundleVersion: "v1", Purpose: CredentialPurposeTLSServer, ConsumerType: CredentialConsumerService, ProfileID: "profile", CompatibilityTargetID: "target"}
}

func TestCredentialProviderSecretValueContracts(t *testing.T) {
	data := CredentialBytes{1, 2, 3}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CredentialBytes
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	copy := decoded.Bytes()
	copy[0] = 9
	if decoded[0] != 1 {
		t.Fatal("Bytes did not return a defensive copy")
	}
	if got := fmt.Sprintf("%s %#v %x", data, data, data); strings.Contains(got, "010203") {
		t.Fatalf("bytes leaked: %s", got)
	}
	var nilBytes *CredentialBytes
	if err := nilBytes.UnmarshalJSON(encoded); err == nil {
		t.Fatal("nil bytes accepted")
	}
	for _, raw := range []string{`1`, `"AQ"`, `"A Q=="`, `"AR=="`} {
		var value CredentialBytes
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	reference := NewCredentialSecretReference("opaque")
	if reference.Reveal() != "opaque" || (CredentialSecretReference{}).Reveal() != "" {
		t.Fatal("reference reveal")
	}
	raw, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	var decodedReference CredentialSecretReference
	if err := json.Unmarshal(raw, &decodedReference); err != nil || decodedReference.Reveal() != "opaque" {
		t.Fatalf("reference decode: %v", err)
	}
	var nilReference *CredentialSecretReference
	if err := nilReference.UnmarshalJSON(raw); err == nil {
		t.Fatal("nil reference accepted")
	}
	if err := json.Unmarshal([]byte(`1`), &decodedReference); err == nil {
		t.Fatal("non-string reference accepted")
	}
	if err := (CredentialSecretReference{}).Validate(); err == nil {
		t.Fatal("empty reference accepted")
	}

	path := NewCredentialProtectedPath("/protected/key")
	if path.Reveal() != "/protected/key" || (CredentialProtectedPath{}).Reveal() != "" {
		t.Fatal("path reveal")
	}
	raw, err = json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	var decodedPath CredentialProtectedPath
	if err := json.Unmarshal(raw, &decodedPath); err != nil {
		t.Fatal(err)
	}
	var nilPath *CredentialProtectedPath
	if err := nilPath.UnmarshalJSON(raw); err == nil {
		t.Fatal("nil path accepted")
	}
	if err := json.Unmarshal([]byte(`1`), &decodedPath); err == nil {
		t.Fatal("non-string path accepted")
	}
	if err := (CredentialProtectedPath{}).Validate(); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestCredentialProviderScopeTargetAndReferenceValidation(t *testing.T) {
	if err := (CredentialOperationScope{}).Validate(); err != nil {
		t.Fatal(err)
	}
	validScope := CredentialOperationScope{OperationID: "operation", RunID: "run", ChainID: "chain", ThrowID: "throw", Target: "target", ListenerID: "listener", NodeID: "node"}
	if err := validScope.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialOperationScope){func(s *CredentialOperationScope) { s.OperationID = " bad" }, func(s *CredentialOperationScope) { s.NodeID = "\n" }} {
		value := validScope
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
	target := validCredentialProviderTarget()
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialProviderTarget){func(v *CredentialProviderTarget) { v.ModuleID = "" }, func(v *CredentialProviderTarget) { v.ProviderID = "" }, func(v *CredentialProviderTarget) { v.ProviderVersion = "" }, func(v *CredentialProviderTarget) { v.DescriptorSHA256 = "bad" }} {
		value := target
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
	reference := CredentialScopedReference{ProviderID: "provider", Reference: NewCredentialSecretReference("reference"), Capabilities: []string{"sign", "decrypt"}}
	if err := reference.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialScopedReference){func(v *CredentialScopedReference) { v.ProviderID = "" }, func(v *CredentialScopedReference) { v.Reference = CredentialSecretReference{} }, func(v *CredentialScopedReference) { v.Capabilities = []string{"x", "x"} }, func(v *CredentialScopedReference) { v.Capabilities = []string{"\n"} }} {
		value := reference
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
}

func TestCredentialStampedDigestEqualityEmpty(t *testing.T) {
	if !credentialStampedMaterialDigestsEqual(nil, nil) {
		t.Fatal("two empty digest sets differ")
	}
}

func TestCredentialProviderMaterialUnion(t *testing.T) {
	if _, err := NewCredentialMaterialBytes(nil); err == nil {
		t.Fatal("empty material accepted")
	}
	bytesValue, err := NewCredentialMaterialBytes([]byte("material"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bytesValue.Reference(); ok {
		t.Fatal("bytes exposed as reference")
	}
	got, ok := bytesValue.Bytes()
	if !ok {
		t.Fatal("bytes variant missing")
	}
	got[0] = 'X'
	again, _ := bytesValue.Bytes()
	if string(again) != "material" {
		t.Fatal("material mutated")
	}
	reference := CredentialScopedReference{ProviderID: "provider", Reference: NewCredentialSecretReference("reference"), Capabilities: []string{"sign"}}
	refValue, err := NewCredentialMaterialReference(reference)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := refValue.Bytes(); ok {
		t.Fatal("reference exposed as bytes")
	}
	gotReference, ok := refValue.Reference()
	if !ok {
		t.Fatal("reference variant missing")
	}
	gotReference.Capabilities[0] = "changed"
	againReference, _ := refValue.Reference()
	if againReference.Capabilities[0] != "sign" {
		t.Fatal("reference mutated")
	}
	if _, err := NewCredentialMaterialReference(CredentialScopedReference{}); err == nil {
		t.Fatal("invalid reference accepted")
	}

	public, err := NewResolvedCredentialMaterial(CredentialProjectionCertificateDER, CredentialMaterialPublic, CredentialEncodingRaw, credentialDigest([]byte("material")), bytesValue)
	if err != nil {
		t.Fatal(err)
	}
	privateRef, err := NewResolvedCredentialMaterial(CredentialProjectionSignerReference, CredentialMaterialPrivateReference, CredentialEncodingRaw, strings.Repeat("a", 64), refValue)
	if err != nil {
		t.Fatal(err)
	}
	for _, material := range []ResolvedCredentialMaterial{public, privateRef} {
		raw, err := json.Marshal(material)
		if err != nil {
			t.Fatal(err)
		}
		var decoded ResolvedCredentialMaterial
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
	}
	if _, err := NewResolvedCredentialMaterial(CredentialProjectionCertificateDER, CredentialMaterialPublic, CredentialEncodingRaw, credentialDigest([]byte("material")), refValue); err == nil {
		t.Fatal("mismatched public reference accepted")
	}
	if _, err := NewResolvedCredentialMaterial(CredentialProjectionSignerReference, CredentialMaterialPrivateReference, CredentialEncodingRaw, strings.Repeat("a", 64), bytesValue); err == nil {
		t.Fatal("mismatched private bytes accepted")
	}
	badDigest := public
	badDigest.SHA256 = strings.Repeat("0", 64)
	if err := badDigest.Validate(); err == nil {
		t.Fatal("bad digest accepted")
	}
	var nilMaterial *ResolvedCredentialMaterial
	if err := nilMaterial.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil material accepted")
	}
	for _, raw := range []string{`{`, `{"projection":"certificate-der","form":"public","encoding":"raw","sha256":"x"}`, `{"projection":"certificate-der","form":"public","encoding":"raw","sha256":"x","data":"bWF0ZXJpYWw=","reference":{}}`, `{"projection":"certificate-der","form":"public","encoding":"raw","sha256":"x","data":"bad"}`, `{"projection":"signer-reference","form":"private-reference","encoding":"raw","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reference":{}}`} {
		var material ResolvedCredentialMaterial
		if err := json.Unmarshal([]byte(raw), &material); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func validProviderMaterial(t *testing.T) ResolvedCredentialMaterial {
	t.Helper()
	data := []byte("material")
	value, err := NewCredentialMaterialBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	material, err := NewResolvedCredentialMaterial(CredentialProjectionCertificateDER, CredentialMaterialPublic, CredentialEncodingRaw, credentialDigest(data), value)
	if err != nil {
		t.Fatal(err)
	}
	return material
}

func TestCredentialProviderDeliveryRequests(t *testing.T) {
	material := validProviderMaterial(t)
	scope := CredentialOperationScope{OperationID: "operation"}
	runtime := CredentialRuntimeRequest{SchemaVersion: CredentialProviderExecutionSchemaV1, Provider: validCredentialProviderTarget(), RequestID: "request", AssignmentID: "assignment", SlotName: "slot", Credential: validResolvedCredentialMetadata(), Material: material, Scope: scope}
	if err := runtime.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialRuntimeRequest){
		func(r *CredentialRuntimeRequest) { r.SchemaVersion = "bad" }, func(r *CredentialRuntimeRequest) { r.RequestID = "" },
		func(r *CredentialRuntimeRequest) { r.Provider.ModuleID = "" }, func(r *CredentialRuntimeRequest) { r.AssignmentID = "" },
		func(r *CredentialRuntimeRequest) { r.SlotName = "" }, func(r *CredentialRuntimeRequest) { r.Credential.ProfileID = "" },
		func(r *CredentialRuntimeRequest) { r.Scope.OperationID = " bad" }, func(r *CredentialRuntimeRequest) { r.Material = ResolvedCredentialMaterial{} },
	} {
		value := runtime
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("runtime accepted %#v", value)
		}
	}

	file := CredentialFile{Projection: CredentialProjectionCertificateDER, Form: CredentialMaterialPublic, Encoding: CredentialEncodingRaw, MediaType: "application/pkix-cert", Path: NewCredentialProtectedPath("/protected/cert"), SHA256: strings.Repeat("a", 64), Size: 1}
	if err := file.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialFile){
		func(f *CredentialFile) { f.Projection = "bad" }, func(f *CredentialFile) { f.Encoding = "" }, func(f *CredentialFile) { f.MediaType = "" },
		func(f *CredentialFile) { f.Path = CredentialProtectedPath{} }, func(f *CredentialFile) { f.SHA256 = "bad" }, func(f *CredentialFile) { f.Size = 0 },
	} {
		value := file
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("file accepted %#v", value)
		}
	}
	files := CredentialFilesRequest{SchemaVersion: CredentialProviderExecutionSchemaV1, Provider: validCredentialProviderTarget(), RequestID: "request", AssignmentID: "assignment", SlotName: "slot", Credential: validResolvedCredentialMetadata(), Files: []CredentialFile{file}, Scope: scope}
	if err := files.Validate(); err != nil {
		t.Fatal(err)
	}
	empty := files
	empty.Files = nil
	if err := empty.Validate(); err == nil {
		t.Fatal("empty files accepted")
	}
	duplicate := files
	duplicate.Files = []CredentialFile{file, file}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate files accepted")
	}
	badFile := files
	badFile.Files[0].Size = 0
	if err := badFile.Validate(); err == nil {
		t.Fatal("bad file accepted")
	}
	for _, mutate := range []func(*CredentialFilesRequest){
		func(r *CredentialFilesRequest) { r.RequestID = "" },
		func(r *CredentialFilesRequest) { r.Provider.ModuleID = "" },
		func(r *CredentialFilesRequest) { r.AssignmentID = "" },
	} {
		candidate := files
		candidate.Files = append([]CredentialFile(nil), files.Files...)
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid files request accepted: %#v", candidate)
		}
	}

	receipt := CredentialDeliveryReceipt{RequestID: "request", ProviderReference: "reference", ReceiptSHA256: strings.Repeat("a", 64)}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []CredentialDeliveryReceipt{{}, {RequestID: "request", ProviderReference: " bad"}, {RequestID: "request", ReceiptSHA256: "bad"}} {
		if err := value.Validate(); err == nil {
			t.Fatalf("receipt accepted %#v", value)
		}
	}
}

func TestCredentialProviderEncodingContract(t *testing.T) {
	material := validProviderMaterial(t)
	request := CredentialEncodingRequest{SchemaVersion: CredentialProviderExecutionSchemaV1, Provider: validCredentialProviderTarget(), RequestID: "request", ProviderID: "provider", ProviderSchema: "v1", OutputForm: CredentialMaterialPublic, MaximumEncodedBytes: 32, Source: material, Scope: CredentialOperationScope{OperationID: "operation"}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialEncodingRequest){
		func(r *CredentialEncodingRequest) { r.SchemaVersion = "bad" }, func(r *CredentialEncodingRequest) { r.Provider.ModuleID = "" },
		func(r *CredentialEncodingRequest) { r.ProviderID = "" }, func(r *CredentialEncodingRequest) { r.ProviderID = "other" },
		func(r *CredentialEncodingRequest) { r.ProviderSchema = "" }, func(r *CredentialEncodingRequest) { r.OutputForm = "bad" },
		func(r *CredentialEncodingRequest) { r.MaximumEncodedBytes = 0 }, func(r *CredentialEncodingRequest) { r.Source = ResolvedCredentialMaterial{} },
		func(r *CredentialEncodingRequest) { r.Scope.OperationID = " bad" },
	} {
		value := request
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("encoding request accepted %#v", value)
		}
	}
	data := CredentialBytes("encoded")
	result := CredentialEncodingResult{RequestID: "request", Form: CredentialMaterialPublic, Encoding: CredentialEncodingRaw, SHA256: credentialDigest(data), Data: data}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CredentialEncodingResult){
		func(r *CredentialEncodingResult) { r.RequestID = "" }, func(r *CredentialEncodingResult) { r.Form = "bad" }, func(r *CredentialEncodingResult) { r.Encoding = "" },
		func(r *CredentialEncodingResult) { r.Data = nil }, func(r *CredentialEncodingResult) { r.SHA256 = "bad" },
	} {
		value := result
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("encoding result accepted %#v", value)
		}
	}
	wrong := result
	wrong.RequestID = "other"
	if err := wrong.ValidateFor(request); err == nil {
		t.Fatal("mismatched result accepted")
	}
	badRequest := request
	badRequest.SchemaVersion = "bad"
	if err := result.ValidateFor(badRequest); err == nil {
		t.Fatal("invalid request accepted")
	}
	badResult := result
	badResult.SHA256 = "bad"
	if err := badResult.ValidateFor(request); err == nil {
		t.Fatal("invalid result accepted")
	}
}

func TestCredentialProviderArtifactUnions(t *testing.T) {
	if _, err := NewCredentialArtifactData(nil); err == nil {
		t.Fatal("empty artifact accepted")
	}
	data, err := NewCredentialArtifactData([]byte("artifact"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := NewCredentialArtifactPath("/protected/artifact")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCredentialArtifactPath(""); err == nil {
		t.Fatal("empty path accepted")
	}
	got, ok := data.Data()
	if !ok {
		t.Fatal("data missing")
	}
	got[0] = 'X'
	again, _ := data.Data()
	if string(again) != "artifact" {
		t.Fatal("artifact mutated")
	}
	if _, ok := data.Path(); ok {
		t.Fatal("data exposed as path")
	}
	if _, ok := path.Data(); ok {
		t.Fatal("path exposed as data")
	}
	if value, ok := path.Path(); !ok || value.Reveal() != "/protected/artifact" {
		t.Fatal("path missing")
	}
	inputData := CredentialArtifactInput{ID: "input", SHA256: credentialDigest([]byte("artifact")), Encoding: CredentialEncodingRaw, Content: data}
	inputPath := CredentialArtifactInput{ID: "input", SHA256: strings.Repeat("a", 64), Encoding: CredentialEncodingRaw, Content: path}
	for _, input := range []CredentialArtifactInput{inputData, inputPath} {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CredentialArtifactInput
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []CredentialArtifactInput{{}, {ID: "input", SHA256: "bad", Encoding: CredentialEncodingRaw, Content: data}, {ID: "input", SHA256: credentialDigest([]byte("artifact")), Encoding: "", Content: data}, {ID: "input", SHA256: strings.Repeat("a", 64), Encoding: CredentialEncodingRaw}} {
		if err := input.Validate(); err == nil {
			t.Fatalf("input accepted %#v", input)
		}
	}
	var nilInput *CredentialArtifactInput
	if err := nilInput.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil input accepted")
	}
	for _, raw := range []string{`{`, `{"id":"x","sha256":"x","encoding":"raw"}`, `{"id":"x","sha256":"x","encoding":"raw","data":"YQ==","path":"/x"}`, `{"id":"x","sha256":"x","encoding":"raw","data":"bad"}`} {
		var value CredentialArtifactInput
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			t.Fatalf("input accepted %s", raw)
		}
	}

	outputData := CredentialArtifactOutput{Name: "output", Encoding: CredentialEncodingRaw, Content: data}
	outputPath := CredentialArtifactOutput{Name: "output", Encoding: CredentialEncodingRaw, Content: path}
	for _, output := range []CredentialArtifactOutput{outputData, outputPath} {
		raw, err := json.Marshal(output)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CredentialArtifactOutput
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
	}
	for _, output := range []CredentialArtifactOutput{{}, {Name: "output", Encoding: "", Content: data}, {Name: "output", Encoding: CredentialEncodingRaw}} {
		if err := output.Validate(); err == nil {
			t.Fatalf("output accepted %#v", output)
		}
	}
	var nilOutput *CredentialArtifactOutput
	if err := nilOutput.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil output accepted")
	}
}

func TestCredentialProviderStampOutputUnion(t *testing.T) {
	content, _ := NewCredentialArtifactData([]byte("artifact"))
	artifact := CredentialArtifactOutput{Name: "output", Encoding: CredentialEncodingRaw, Content: content}
	artifactOutput, err := NewCredentialStampArtifactOutput(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCredentialStampArtifactOutput(CredentialArtifactOutput{}); err == nil {
		t.Fatal("invalid artifact accepted")
	}
	deployment := CredentialDeploymentOutput{Reference: "deployment", Receipt: CredentialBytes("receipt")}
	deploymentOutput, err := NewCredentialStampDeploymentOutput(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCredentialStampDeploymentOutput(CredentialDeploymentOutput{}); err == nil {
		t.Fatal("invalid deployment accepted")
	}
	for _, output := range []CredentialStampOutput{artifactOutput, deploymentOutput} {
		raw, err := json.Marshal(output)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CredentialStampOutput
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
	}
	gotArtifact, ok := artifactOutput.Artifact()
	if !ok || gotArtifact.Name != "output" {
		t.Fatal("artifact variant")
	}
	if _, ok := artifactOutput.Deployment(); ok {
		t.Fatal("artifact exposed as deployment")
	}
	gotDeployment, ok := deploymentOutput.Deployment()
	if !ok {
		t.Fatal("deployment variant")
	}
	gotDeployment.Receipt[0] = 'X'
	again, _ := deploymentOutput.Deployment()
	if string(again.Receipt) != "receipt" {
		t.Fatal("receipt mutated")
	}
	if _, ok := deploymentOutput.Artifact(); ok {
		t.Fatal("deployment exposed as artifact")
	}
	if err := (CredentialStampOutput{}).Validate(); err == nil {
		t.Fatal("unset output accepted")
	}
	var nilOutput *CredentialStampOutput
	if err := nilOutput.UnmarshalJSON(nil); err == nil {
		t.Fatal("nil stamp output accepted")
	}
	for _, raw := range []string{`{`, `{}`, `{"artifact":{},"deployment":{}}`, `{"artifact":{}}`, `{"deployment":{}}`} {
		var output CredentialStampOutput
		if err := json.Unmarshal([]byte(raw), &output); err == nil {
			t.Fatalf("stamp output accepted %s", raw)
		}
	}
	for _, value := range []CredentialDeploymentOutput{{Reference: "", Receipt: CredentialBytes("x")}, {Reference: "x"}} {
		if err := value.Validate(); err == nil {
			t.Fatalf("deployment accepted %#v", value)
		}
	}
	for _, value := range []CredentialStampTargetResolution{CredentialStampTargetUnchanged, CredentialStampTargetTranslated} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := CredentialStampTargetResolution("bad").Validate(); err == nil {
		t.Fatal("bad resolution accepted")
	}
	digest := CredentialStampedMaterialDigest{Projection: CredentialProjectionCertificateDER, Reference: "certificate", SHA256: strings.Repeat("a", 64)}
	if err := digest.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []CredentialStampedMaterialDigest{{}, {Projection: CredentialProjectionCertificateDER, Reference: "", SHA256: strings.Repeat("a", 64)}, {Projection: CredentialProjectionCertificateDER, Reference: "x", SHA256: "bad"}} {
		if err := value.Validate(); err == nil {
			t.Fatalf("digest accepted %#v", value)
		}
	}
	if err := validateCredentialStampedMaterialDigests(nil); err == nil {
		t.Fatal("empty digests accepted")
	}
	if err := validateCredentialStampedMaterialDigests([]CredentialStampedMaterialDigest{digest, digest}); err == nil {
		t.Fatal("duplicate digests accepted")
	}
	if !credentialStampedMaterialDigestsEqual([]CredentialStampedMaterialDigest{digest}, []CredentialStampedMaterialDigest{digest}) {
		t.Fatal("equal digests differ")
	}
	if credentialStampedMaterialDigestsEqual(nil, []CredentialStampedMaterialDigest{digest}) {
		t.Fatal("length mismatch equal")
	}
	other := digest
	other.SHA256 = strings.Repeat("b", 64)
	if credentialStampedMaterialDigestsEqual([]CredentialStampedMaterialDigest{other}, []CredentialStampedMaterialDigest{digest}) {
		t.Fatal("digest mismatch equal")
	}
}

func TestCredentialProviderRejectsCorruptedSealedUnions(t *testing.T) {
	reference := CredentialScopedReference{ProviderID: "provider", Reference: NewCredentialSecretReference("reference")}
	tooMany := reference
	tooMany.Capabilities = make([]string, maximumCredentialReferenceCapabilities+1)
	if err := tooMany.Validate(); err == nil {
		t.Fatal("oversized reference capabilities accepted")
	}

	validBytes, _ := NewCredentialMaterialBytes([]byte("material"))
	validReference, _ := NewCredentialMaterialReference(reference)
	base := ResolvedCredentialMaterial{Projection: CredentialProjectionCertificateDER, Encoding: CredentialEncodingRaw, SHA256: credentialDigest([]byte("material")), form: CredentialMaterialPublic, value: validBytes}
	corruptMaterials := []ResolvedCredentialMaterial{
		{Projection: CredentialProjectionCertificateDER, Encoding: "", SHA256: strings.Repeat("a", 64), form: CredentialMaterialPublic, value: validBytes},
		{Projection: CredentialProjectionCertificateDER, Encoding: CredentialEncodingRaw, SHA256: "bad", form: CredentialMaterialPublic, value: validBytes},
		{Projection: CredentialProjectionSignerReference, Encoding: CredentialEncodingRaw, SHA256: strings.Repeat("a", 64), form: CredentialMaterialPrivateReference, value: validBytes},
		{Projection: CredentialProjectionSignerReference, Encoding: CredentialEncodingRaw, SHA256: strings.Repeat("a", 64), form: CredentialMaterialPrivateReference, value: CredentialMaterialValue{kind: credentialMaterialValueReference, data: []byte{1}, reference: reference}},
		{Projection: CredentialProjectionCertificateDER, Encoding: CredentialEncodingRaw, SHA256: strings.Repeat("a", 64), form: CredentialMaterialPublic, value: validReference},
		{Projection: CredentialProjectionCertificateDER, Encoding: CredentialEncodingRaw, SHA256: strings.Repeat("a", 64), form: CredentialMaterialPublic, value: CredentialMaterialValue{kind: credentialMaterialValueBytes, data: []byte{1}, reference: reference}},
	}
	for index, material := range corruptMaterials {
		if err := material.Validate(); err == nil {
			t.Fatalf("corrupt material %d accepted", index)
		}
		if _, err := json.Marshal(material); err == nil {
			t.Fatalf("corrupt material %d marshaled", index)
		}
	}
	if _, err := json.Marshal(ResolvedCredentialMaterial{form: CredentialMaterialForm("unknown")}); err == nil {
		t.Fatal("unknown material form marshaled")
	}
	if _, err := NewResolvedCredentialMaterial(CredentialProjectionCertificateDER, CredentialMaterialPublic, CredentialEncodingRaw, base.SHA256, CredentialMaterialValue{kind: credentialMaterialValueBytes}); err == nil {
		t.Fatal("empty byte material constructed")
	}
	if cloned := (ResolvedCredentialMaterial{}).clone(); cloned.value.kind != credentialMaterialValueUnset {
		t.Fatal("unset material clone changed its variant")
	}

	data := CredentialArtifactContent{kind: credentialArtifactContentData, data: []byte("artifact")}
	path := CredentialArtifactContent{kind: credentialArtifactContentPath, path: NewCredentialProtectedPath("/protected/artifact")}
	corruptContents := []CredentialArtifactContent{
		{kind: credentialArtifactContentData},
		{kind: credentialArtifactContentData, data: []byte{1}, path: NewCredentialProtectedPath("/also-path")},
		{kind: credentialArtifactContentPath, data: []byte{1}, path: NewCredentialProtectedPath("/path")},
		{kind: credentialArtifactContentPath},
		{kind: 99},
	}
	for index, content := range corruptContents {
		input := CredentialArtifactInput{ID: "input", SHA256: credentialDigest(content.data), Encoding: CredentialEncodingRaw, Content: content}
		if err := input.Validate(); err == nil {
			t.Fatalf("corrupt input content %d accepted", index)
		}
		output := CredentialArtifactOutput{Name: "output", Encoding: CredentialEncodingRaw, Content: content}
		if err := output.Validate(); err == nil {
			t.Fatalf("corrupt output content %d accepted", index)
		}
	}
	if _, err := marshalCredentialArtifact(credentialArtifactWire{}, CredentialArtifactContent{kind: credentialArtifactContentData}); err == nil {
		t.Fatal("empty artifact data marshaled")
	}
	if _, err := marshalCredentialArtifact(credentialArtifactWire{}, CredentialArtifactContent{kind: credentialArtifactContentPath, data: []byte{1}, path: NewCredentialProtectedPath("/path")}); err == nil {
		t.Fatal("path artifact with data marshaled")
	}
	if _, err := marshalCredentialArtifact(credentialArtifactWire{}, CredentialArtifactContent{kind: credentialArtifactContentPath}); err == nil {
		t.Fatal("empty artifact path marshaled")
	}
	if _, err := marshalCredentialArtifact(credentialArtifactWire{}, CredentialArtifactContent{kind: 99}); err == nil {
		t.Fatal("unknown artifact variant marshaled")
	}
	_ = data
	_ = path
}

func TestCredentialProviderRejectsCorruptedStampOutputs(t *testing.T) {
	content, _ := NewCredentialArtifactData([]byte("artifact"))
	artifact := CredentialArtifactOutput{Name: "output", Encoding: CredentialEncodingRaw, Content: content}
	deployment := CredentialDeploymentOutput{Reference: "deployment", Receipt: CredentialBytes("receipt")}
	values := []CredentialStampOutput{
		{kind: credentialStampOutputArtifact, artifact: artifact, deployment: deployment},
		{kind: credentialStampOutputDeployment, deployment: deployment, artifact: CredentialArtifactOutput{Name: "unexpected"}},
		{kind: credentialStampOutputDeployment, deployment: deployment, artifact: CredentialArtifactOutput{Encoding: "unexpected"}},
		{kind: credentialStampOutputDeployment, deployment: deployment, artifact: CredentialArtifactOutput{Content: content}},
		{kind: 99},
	}
	for index, value := range values {
		if err := value.Validate(); err == nil {
			t.Fatalf("corrupt stamp output %d accepted", index)
		}
		if _, err := json.Marshal(value); err == nil {
			t.Fatalf("corrupt stamp output %d marshaled", index)
		}
	}
}

func validCredentialStampExecution(t *testing.T) (CredentialStampExecutionRequest, CredentialStampExecutionResult) {
	t.Helper()
	encoded, err := json.Marshal(validCredentialProviderParams(credentialRPCStampMethod))
	if err != nil {
		t.Fatal(err)
	}
	var request CredentialStampExecutionRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	result, err := (fakeMeshModule{}).StampCredential(request)
	if err != nil {
		t.Fatal(err)
	}
	return request, result
}

func TestCredentialProviderStampExecutionValidationFailures(t *testing.T) {
	request, result := validCredentialStampExecution(t)
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	requestMutations := []func(*CredentialStampExecutionRequest){
		func(r *CredentialStampExecutionRequest) { r.SchemaVersion = "bad" },
		func(r *CredentialStampExecutionRequest) { r.Provider.ModuleID = "" },
		func(r *CredentialStampExecutionRequest) { r.StampID = "" },
		func(r *CredentialStampExecutionRequest) { r.Request.AssignmentID = "" },
		func(r *CredentialStampExecutionRequest) { r.Input.ID = "" },
		func(r *CredentialStampExecutionRequest) { r.Material = ResolvedCredentialMaterial{} },
		func(r *CredentialStampExecutionRequest) { r.Request.Material = CredentialStampMaterial{} },
		func(r *CredentialStampExecutionRequest) { r.Material.Projection = CredentialProjectionPublicKeySPKI },
		func(r *CredentialStampExecutionRequest) { r.ExpectedDigests = nil },
		func(r *CredentialStampExecutionRequest) { r.Scope.OperationID = " bad" },
	}
	for index, mutate := range requestMutations {
		candidate := request
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("request mutation %d accepted", index)
		}
	}
	formMismatch := request
	formMismatch.Request.Material = CredentialStampMaterial{
		Projection: CredentialProjectionLiteralReference,
		LiteralReference: &CredentialLiteralMaterialReference{
			Reference: "literal", SHA256: strings.Repeat("a", 64), Form: CredentialMaterialPublic,
		},
	}
	privateValue, err := NewCredentialMaterialBytes([]byte("private"))
	if err != nil {
		t.Fatal(err)
	}
	formMismatch.Material, err = NewResolvedCredentialMaterial(
		CredentialProjectionLiteralReference, CredentialMaterialPrivateBytes,
		CredentialEncodingRaw, credentialDigest([]byte("private")), privateValue,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := formMismatch.Request.Validate(); err != nil {
		t.Fatalf("form-mismatch request fixture is invalid: %v", err)
	}
	if err := formMismatch.Material.Validate(); err != nil {
		t.Fatalf("form-mismatch resolved fixture is invalid: %v", err)
	}
	if formMismatch.Material.Projection != formMismatch.Request.Material.Projection {
		t.Fatal("form-mismatch fixture also mismatches projection")
	}
	if err := formMismatch.Validate(); err == nil {
		t.Fatal("resolved material with a mismatched form was accepted")
	}
	projectionMismatch := formMismatch
	projectionMismatch.Material.Projection = CredentialProjectionProviderEncoding
	if err := projectionMismatch.Material.Validate(); err != nil {
		t.Fatalf("projection-mismatch resolved fixture is invalid: %v", err)
	}
	if err := projectionMismatch.Validate(); err == nil {
		t.Fatal("resolved material with a mismatched projection was accepted")
	}

	resultMutations := []func(*CredentialStampExecutionResult){
		func(r *CredentialStampExecutionResult) { r.StampID = "" },
		func(r *CredentialStampExecutionResult) { r.Output = CredentialStampOutput{} },
		func(r *CredentialStampExecutionResult) { r.TargetResolution = "bad" },
		func(r *CredentialStampExecutionResult) { r.ResolvedTarget = CredentialStampTarget{} },
		func(r *CredentialStampExecutionResult) { r.BytesWritten = "bad" },
		func(r *CredentialStampExecutionResult) { r.BytesWritten = "0" },
		func(r *CredentialStampExecutionResult) { r.MaterialDigests = nil },
	}
	for index, mutate := range resultMutations {
		candidate := result
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("result mutation %d accepted", index)
		}
	}

	badRequest := request
	badRequest.SchemaVersion = "bad"
	if err := result.ValidateFor(badRequest); err == nil {
		t.Fatal("result accepted an invalid request")
	}
	badResult := result
	badResult.StampID = ""
	if err := badResult.ValidateFor(request); err == nil {
		t.Fatal("invalid result matched request")
	}
	wrongID := result
	wrongID.StampID = "other"
	if err := wrongID.ValidateFor(request); err == nil {
		t.Fatal("wrong stamp id matched request")
	}
	wrongCount := result
	wrongCount.BytesWritten = "2"
	if err := wrongCount.ValidateFor(request); err == nil {
		t.Fatal("wrong byte count matched request")
	}
	wrongTarget := result
	wrongTarget.ResolvedTarget = CredentialStampTarget{Kind: CredentialStampTargetNamedSlot, NamedSlot: &CredentialNamedSlotTarget{Name: "other"}}
	if err := wrongTarget.ValidateFor(request); err == nil {
		t.Fatal("wrong unchanged target matched request")
	}
	translated := result
	translated.TargetResolution = CredentialStampTargetTranslated
	if err := translated.ValidateFor(request); err != nil {
		t.Fatalf("translated target rejected: %v", err)
	}
	wrongDigests := result
	wrongDigests.MaterialDigests = append([]CredentialStampedMaterialDigest(nil), result.MaterialDigests...)
	wrongDigests.MaterialDigests[0].SHA256 = strings.Repeat("b", 64)
	if err := wrongDigests.ValidateFor(request); err == nil {
		t.Fatal("wrong material digests matched request")
	}
}

func TestCredentialProviderDirectJSONFailures(t *testing.T) {
	var material ResolvedCredentialMaterial
	if err := material.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("malformed material accepted")
	}
	if err := material.UnmarshalJSON([]byte(`{"projection":"signer-reference","form":"private-reference","encoding":"raw","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reference":1}`)); err == nil {
		t.Fatal("malformed material reference accepted")
	}
	if err := material.UnmarshalJSON([]byte(`{"projection":"signer-reference","form":"private-reference","encoding":"raw","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","data":"bWF0ZXJpYWw="}`)); err == nil {
		t.Fatal("form-mismatched material accepted")
	}
	var input CredentialArtifactInput
	if err := input.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("malformed input accepted")
	}
	if err := input.UnmarshalJSON([]byte(`{"id":"","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","encoding":"raw","data":"YQ=="}`)); err == nil {
		t.Fatal("invalid decoded input accepted")
	}
	if _, err := json.Marshal(CredentialArtifactInput{}); err == nil {
		t.Fatal("invalid input marshaled")
	}
	var output CredentialArtifactOutput
	if err := output.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("malformed output accepted")
	}
	if _, err := credentialArtifactContentFromWire(credentialArtifactWire{Path: json.RawMessage(`1`)}); err == nil {
		t.Fatal("non-string artifact path accepted")
	}
	if err := output.UnmarshalJSON([]byte(`{"name":"","encoding":"raw","data":"YQ=="}`)); err == nil {
		t.Fatal("invalid decoded output accepted")
	}
	if _, err := json.Marshal(CredentialArtifactOutput{}); err == nil {
		t.Fatal("invalid output marshaled")
	}
	var stamp CredentialStampOutput
	if err := stamp.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("malformed stamp output accepted")
	}
	if err := stamp.UnmarshalJSON([]byte(`{"artifact":{}}`)); err == nil {
		t.Fatal("invalid artifact stamp output accepted")
	}
	if err := stamp.UnmarshalJSON([]byte(`{"deployment":1}`)); err == nil {
		t.Fatal("invalid deployment stamp output accepted")
	}
}
