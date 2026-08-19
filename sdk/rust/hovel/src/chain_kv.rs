use crate::json::Value;

#[derive(Clone, Debug, Default)]
pub struct ChainKVBinding {
    pub key: String,
    pub config_key: String,
    pub step_id: String,
    pub description: String,
    pub required: bool,
}

impl ChainKVBinding {
    pub fn new(key: &str) -> ChainKVBinding {
        ChainKVBinding {
            key: key.to_string(),
            ..Default::default()
        }
    }

    fn to_value(&self) -> Value {
        Value::object(vec![
            ("key", Value::from(self.key.as_str())),
            ("configKey", Value::from(self.config_key.as_str())),
            ("stepId", Value::from(self.step_id.as_str())),
            ("description", Value::from(self.description.as_str())),
            ("required", Value::Bool(self.required)),
        ])
    }
}

#[derive(Clone, Debug, Default)]
pub struct ChainKVContract {
    pub requires: Vec<ChainKVBinding>,
    pub produces: Vec<ChainKVBinding>,
}

impl ChainKVContract {
    pub(crate) fn to_value(&self) -> Value {
        Value::object(vec![
            (
                "requires",
                Value::Array(self.requires.iter().map(ChainKVBinding::to_value).collect()),
            ),
            (
                "produces",
                Value::Array(self.produces.iter().map(ChainKVBinding::to_value).collect()),
            ),
        ])
    }
}

#[derive(Clone, Debug)]
pub struct ChainKV {
    available: bool,
    target: String,
    revision: u64,
    entries: Vec<(String, String)>,
    operations: Vec<Value>,
}

impl ChainKV {
    pub(crate) fn from_params(target: &str, params: &Value) -> ChainKV {
        let payload = params.get("chainKV");
        let available = matches!(payload, Some(Value::Object(_)));
        let revision = payload
            .and_then(|value| value.get("revision"))
            .and_then(Value::as_f64)
            .unwrap_or(0.0) as u64;
        let entries = payload
            .and_then(|value| value.get("entries"))
            .and_then(Value::as_object)
            .unwrap_or(&[])
            .iter()
            .filter_map(|(key, value)| value.as_str().map(|text| (key.clone(), text.to_string())))
            .collect();
        ChainKV {
            available,
            target: target.to_string(),
            revision,
            entries,
            operations: Vec::new(),
        }
    }

    pub fn available(&self) -> bool {
        self.available
    }
    pub fn revision(&self) -> u64 {
        self.revision
    }

    pub fn get(&self, key: &str) -> Option<&str> {
        let key = self.expand(key);
        self.entries
            .iter()
            .find(|(candidate, _)| candidate == &key)
            .map(|(_, value)| value.as_str())
    }

    pub fn exists(&self, key: &str) -> bool {
        self.get(key).is_some()
    }

    pub fn set(&mut self, key: &str, value: &str) -> Result<(), String> {
        if !self.available {
            return Err("hovel: chain kv is not available in this runtime".into());
        }
        let key = self.expand(key);
        if key.trim().is_empty() {
            return Err("hovel: chain kv key is required".into());
        }
        if let Some((_, current)) = self
            .entries
            .iter_mut()
            .find(|(candidate, _)| candidate == &key)
        {
            *current = value.to_string();
        } else {
            self.entries.push((key.clone(), value.to_string()));
        }
        self.operations.push(Value::object(vec![
            ("operation", Value::from("set")),
            ("key", Value::Str(key)),
            ("value", Value::from(value)),
        ]));
        Ok(())
    }

    pub fn delete(&mut self, key: &str) -> Result<(), String> {
        if !self.available {
            return Err("hovel: chain kv is not available in this runtime".into());
        }
        let key = self.expand(key);
        if key.trim().is_empty() {
            return Err("hovel: chain kv key is required".into());
        }
        self.entries.retain(|(candidate, _)| candidate != &key);
        self.operations.push(Value::object(vec![
            ("operation", Value::from("delete")),
            ("key", Value::Str(key)),
        ]));
        Ok(())
    }

    pub(crate) fn mutations(&self) -> Option<Value> {
        if !self.available || self.operations.is_empty() {
            return None;
        }
        Some(Value::object(vec![
            ("baseRevision", Value::Num(self.revision as f64)),
            ("operations", Value::Array(self.operations.clone())),
        ]))
    }

    fn expand(&self, key: &str) -> String {
        key.replace("{target}", &percent_encode(&self.target))
    }
}

fn percent_encode(value: &str) -> String {
    let mut out = String::new();
    for byte in value.as_bytes() {
        if byte.is_ascii_alphanumeric() || matches!(*byte, b'-' | b'_' | b'.' | b'~') {
            out.push(*byte as char);
        } else {
            out.push_str(&format!("%{byte:02X}"));
        }
    }
    out
}
