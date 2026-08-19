from __future__ import annotations

from typing import ClassVar

from hovel_sdk import Context, HovelModule, Requirement, Result, serve


class ContractProbe(HovelModule):
    name = "contract-probe"
    version = "v0.3.2-compat"
    module_type = "survey"
    summary = "Deterministic SDK compatibility probe."
    tags: ClassVar[tuple[str, ...]] = ("compat", "python")
    target_config: ClassVar[tuple[Requirement, ...]] = (Requirement("target.host", "host", description="Target host."),)

    def run(self, ctx: Context) -> Result:
        value = ctx.input("probe.value", "default")
        return Result.ok({"echo": value, "target": ctx.target}, summary="probe complete")


if __name__ == "__main__":
    serve(ContractProbe())
