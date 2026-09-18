#!/usr/bin/env python3
"""Refresh the Output comments of the examples that assert help text.

An example's Output comment is a golden, and the help text it holds changes
whenever a flag or a default changes. Rewriting those by hand invites a typo
that reads as a real difference. This blanks each one, runs the example, and
puts back exactly what the example printed.

    python3 scripts/regolden.py

It fails if an example did not run, so a blanked golden can never be left
behind.
"""

import pathlib
import re
import subprocess
import sys

# (module directory, file, example) for every example whose Output is help text.
GOLDENS = [
    (".", "example_test.go", "ExampleInstaller_Run"),
    ("skillcobra", "skillcobra_test.go", "ExampleCommand"),
    ("skillurfavev3", "skillurfavev3_test.go", "ExampleCommand"),
    ("skillurfavev2", "skillurfavev2_test.go", "ExampleCommand"),
]

PLACEHOLDER = "\t// __GOLDEN__"


def refresh(directory: str, filename: str, example: str) -> None:
    path = pathlib.Path(directory) / filename
    source = path.read_text()

    blanked = re.sub(
        r"(func " + example + r"\(\) \{.*?\t// Output:\n)(?:\t//.*\n)*(\})",
        r"\1" + PLACEHOLDER + r"\n\2",
        source,
        flags=re.S,
    )
    if PLACEHOLDER not in blanked:
        sys.exit(f"{path}: {example} has no Output comment to refresh")
    path.write_text(blanked)

    result = subprocess.run(
        ["go", "test", "-run", example, "."],
        cwd=directory,
        capture_output=True,
        text=True,
    )
    if "got:\n" not in result.stdout:
        path.write_text(source)  # leave the tree as it was found
        sys.exit(f"{path}: {example} printed nothing\n{result.stdout}{result.stderr}")

    printed = result.stdout[
        result.stdout.index("got:\n") + len("got:\n") : result.stdout.index("\nwant:")
    ]
    golden = "\n".join(("\t// " + line).rstrip() for line in printed.split("\n"))
    path.write_text(path.read_text().replace(PLACEHOLDER, golden))
    print(f"{path}: {example}")


def main() -> None:
    for directory, filename, example in GOLDENS:
        refresh(directory, filename, example)
    subprocess.run(["gofmt", "-w", "."], check=True)


if __name__ == "__main__":
    main()
