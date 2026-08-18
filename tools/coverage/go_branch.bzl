"""Rules for generating branch-instrumented shadow Go sources."""

def _go_branch_source_impl(ctx):
    output = ctx.actions.declare_file(ctx.label.name + "/" + ctx.file.src.basename)
    args = ctx.actions.args()
    args.add("--input", ctx.file.src)
    args.add("--output", output)
    args.add("--logical-path", ctx.file.src.short_path)
    ctx.actions.run(
        executable = ctx.executable._instrumenter,
        inputs = [ctx.file.src],
        outputs = [output],
        arguments = [args],
        mnemonic = "GoBranchInstrument",
        progress_message = "Instrumenting Go branches in %{input}",
    )
    return [DefaultInfo(files = depset([output]))]

_go_branch_source = rule(
    implementation = _go_branch_source_impl,
    attrs = {
        "src": attr.label(allow_single_file = [".go"], mandatory = True),
        "_instrumenter": attr.label(
            default = "//tools/coverage:go_branch_instrument",
            executable = True,
            cfg = "exec",
        ),
    },
)

def go_branch_sources(name, srcs):
    """Creates one generated target per source.

    Args:
      name: Prefix for the generated targets.
      srcs: Go source paths to instrument.

    Returns:
      Labels for the generated instrumented sources.
    """
    labels = []
    for src in srcs:
        stem = src[:-3].replace("/", "_")
        target = name + "_" + stem
        _go_branch_source(name = target, src = src)
        labels.append(":" + target)
    return labels
