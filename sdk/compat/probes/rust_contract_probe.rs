use hovel::json::Value;
use hovel::{Context, Info, Module, ModuleType, Outcome, Requirement, Schema};

struct ContractProbe;

impl Module for ContractProbe {
    fn info(&self) -> Info {
        Info {
            name: "contract-probe".into(),
            version: "v0.3.2-compat".into(),
            module_type: ModuleType::Survey,
            summary: "Deterministic SDK compatibility probe.".into(),
            description: String::new(),
            tags: vec!["compat".into(), "rust".into()],
            discovery_context: Vec::new(),
        }
    }

    fn schema(&self) -> Schema {
        Schema {
            target_config: vec![Requirement::new("target.host", "host", "Target host.")],
            ..Schema::default()
        }
    }

    fn run(&self, ctx: &mut Context) -> Outcome {
        let value = ctx.input_str("probe.value", "default");
        Outcome::ok(vec![
            ("echo".into(), Value::from(value.as_str())),
            ("target".into(), Value::from(ctx.target.as_str())),
        ])
        .with_summary("probe complete")
    }
}

fn main() { hovel::serve(ContractProbe); }
