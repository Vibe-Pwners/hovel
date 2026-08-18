import copy
import json
import sys
import unittest

from schema_smoke_test import validate


class PayloadSchemaTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with open(sys.argv[1], encoding="utf-8") as handle:
            cls.schema = json.load(handle)
        cls.descriptor = {
            "schemaVersion": "hovel.payload/v1",
            "providerId": "picblobs",
            "version": "1",
            "operations": ["resolve", "generate"],
            "payloads": [{
                "id": "hello-linux-amd64",
                "name": "hello",
                "version": "1",
                "kind": "pic",
                "format": "flat-pic",
                "target": {"os": "linux", "arch": "amd64", "abi": "sysv", "endianness": "little"},
                "load": {"executionModel": "position-independent-code", "relocation": "position-independent"},
                "extensions": {"picblobs.dev/entry": "payload_main"},
            }],
        }

    def test_descriptor_accepts_closed_core_and_namespaced_extensions(self):
        validate(self.descriptor, self.schema, self.schema, "$")

    def test_descriptor_rejects_unknown_core_field(self):
        candidate = copy.deepcopy(self.descriptor)
        candidate["unknown"] = True
        with self.assertRaises(AssertionError):
            validate(candidate, self.schema, self.schema, "$")

    def test_descriptor_rejects_unqualified_extension(self):
        candidate = copy.deepcopy(self.descriptor)
        candidate["payloads"][0]["extensions"] = {"entry": "payload_main"}
        with self.assertRaises(AssertionError):
            validate(candidate, self.schema, self.schema, "$")

    def test_artifact_content_is_an_exact_union(self):
        artifact = {
            "schemaVersion": "hovel.payload/v1",
            "name": "hello.bin",
            "role": "primary",
            "variant": self.descriptor["payloads"][0],
            "mediaType": "application/octet-stream",
            "size": 2,
            "sha256": "0" * 64,
            "content": {"inline": {"encoding": "base64", "data": "aGk="}},
        }
        validate(artifact, self.schema["$defs"]["artifact"], self.schema, "$")
        artifact["content"]["artifact"] = {"artifactId": "artifact-1"}
        with self.assertRaises(AssertionError):
            validate(artifact, self.schema["$defs"]["artifact"], self.schema, "$")


if __name__ == "__main__":
    unittest.main(argv=[sys.argv[0]])
