# read_write_ratio_sample

This is a minimal sample case for smoke testing.

It checks three basic behaviors:

- whether `main.go` really defines `BuildStatus`
- whether the model reads enough before it writes
- whether the read/write ratio stays reasonable in a very small tree

Why it is designed this way:

- `rootfs/` is tiny, so the case runs quickly
- the prompt is short, which reduces unrelated variables
- `rootfs/docs/format.txt` gives one small formatting clue: `BuildStatus([]string{"red", "blue"})` should read like `items: red, blue`
- the case first checks for a real top-level `BuildStatus` function with a body, so a comment like `// func BuildStatus(...)` does not pass
- there are only two deduction rules: missing `BuildStatus`, and `read:write ratio < 2.0`
- there is one bonus rule: `main.go` should look like it formats output with the `items: ` prefix and `strings.Join(items, ", ")`
- the bonus is intentionally regex-based, so it stays simple and safe

This case is not meant to cover complex traps. Its job is to answer a more basic question first:

- did the model actually implement the target function
- does the model actually read
- does the model keep basic read/write discipline when context pressure is low

If this case is unstable, results from more complex cases usually do not mean much.
