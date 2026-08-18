package main

import "github.com/vibepwners/hovel/sdk/go/hovel"

type contractProbe struct{}

func (contractProbe) Info() hovel.Info {
	return hovel.Info{Name: "contract-probe", Version: "v0.3.2-compat", Type: hovel.TypeSurvey, Summary: "Deterministic SDK compatibility probe.", Tags: []string{"compat", "go"}}
}

func (contractProbe) Schema() hovel.Schema {
	return hovel.Schema{TargetConfig: []hovel.Requirement{hovel.Req("target.host", "host", "Target host.")}}
}

func (contractProbe) Run(ctx *hovel.Context) (hovel.Result, error) {
	return hovel.Ok(map[string]any{"echo": ctx.InputString("probe.value", "default"), "target": ctx.Target}, hovel.WithSummary("probe complete")), nil
}

func main() { hovel.Serve(contractProbe{}) }
