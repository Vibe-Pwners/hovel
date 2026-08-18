import unittest

from hovel_sdk import ChainKV, Context

from hovel_example_survey.module import MockSurvey


class MockSurveyTest(unittest.TestCase):
    def test_survey_returns_facts(self) -> None:
        chain_kv = ChainKV("mock://router-01", {"revision": 0, "entries": {}})
        result = MockSurvey().run(
            Context(
                run_id="run-1",
                module_id="mock-survey",
                target="mock://router-01",
                target_config={"target.host": "router-01", "target.port": "22"},
                chain_kv=chain_kv,
            ),
        )
        self.assertEqual(result.status, "succeeded")
        self.assertEqual(result.outputs["facts"]["host"], "router-01")
        self.assertEqual(chain_kv.get("survey/{target}/port"), "22")
        mutations = chain_kv.to_rpc()
        self.assertIsNotNone(mutations)
        assert mutations is not None
        self.assertEqual(mutations["operations"][0]["value"], "22")

    def test_survey_declares_target_configuration(self) -> None:
        schema = MockSurvey().module_schema()
        self.assertEqual(schema["chainConfig"], [])
        self.assertEqual(schema["targetConfig"][0]["key"], "target.host")
        self.assertEqual(schema["chainKV"]["produces"][0]["key"], "survey/{target}/port")


if __name__ == "__main__":
    unittest.main()
