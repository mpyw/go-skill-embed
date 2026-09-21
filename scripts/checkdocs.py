#!/usr/bin/env python3
"""Compile every Go block in the documents.

A block in a document is hand written, so it drifts when a signature or a type
changes and nothing says so. This extracts each one, wraps it if it is a
fragment, and builds it against the real modules.

    python3 scripts/checkdocs.py

A block it cannot classify is reported and counted as a failure, so a new
shape cannot slip through unchecked.
"""

from __future__ import annotations

import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

REPO = pathlib.Path(__file__).resolve().parent.parent
DOCUMENTS = ["README.md", "skills/go-skill-embed-adoption/SKILL.md"]

# Every read and write is UTF-8. The documents are, a block carries whatever
# they carry, and Windows would otherwise use the locale's encoding.
UTF8 = {"encoding": "utf-8"}

FENCE = re.compile(r"^```go\n(.*?)^```", re.M | re.S)

# Everything a block may reach for, so that `go mod tidy` keeps the requires.
DEPS = """package deps

import (
	_ "github.com/mpyw/go-skill-embed"
	_ "github.com/mpyw/go-skill-embed/skillcobra"
	_ "github.com/mpyw/go-skill-embed/skillurfavev2"
	_ "github.com/mpyw/go-skill-embed/skillurfavev3"
	_ "github.com/spf13/cobra"
	_ "github.com/urfave/cli/v3"
	_ "golang.org/x/tools/go/analysis"
	_ "golang.org/x/tools/go/analysis/singlechecker"
)
"""

# What every block is given, so that it reads as it does in the document.
HEADER = """package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	cli "github.com/urfave/cli/v3"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"

	skillembed "github.com/mpyw/go-skill-embed"
	"github.com/mpyw/go-skill-embed/skillcobra"
	"github.com/mpyw/go-skill-embed/skillurfavev3"
)

var (
	ctx     = context.Background()
	version = "v0"
	root    = &cobra.Command{}
	o       skillembed.InstallOptions
	mylint  = struct{ Analyzer *analysis.Analyzer }{}
)

func report(...any) {}

var _ = []any{
	errors.Is, flag.Parse, fmt.Println, os.Args, cli.Command{}, embed.FS{},
	skillcobra.Command, skillurfavev3.Command, singlechecker.Main,
	skillembed.NewInstaller, version, mylint, o, report, ctx, root,
}
"""

# A statement block has no installer of its own, so it is lent one.
AUX = """
//go:embed skills
var auxFS embed.FS

var skills = skillembed.NewInstaller(skillembed.MustSkillsFromFS(auxFS, "skills"))
"""

DECLARED = re.compile(r"^\s*([\w, ]+?)\s*:=", re.M)


def go_binary() -> str:
    """The real go, resolved from inside the repository.

    mise puts a shim on the PATH, and the shim reads the pinned version from
    the working directory upwards. The scaffold is built in a temporary
    directory, where there is no mise.toml and the shim has nothing to resolve.

    Falling back to the bare name would put that shim back, and it fails with
    "No version is set for shim: go", which says nothing about this. The
    failure is reported here instead.
    """
    try:
        done = subprocess.run(["go", "env", "GOROOT"], cwd=REPO, capture_output=True, text=True)
    except OSError as err:
        sys.exit(f"checkdocs: cannot run go: {err}")
    root = done.stdout.strip()
    if done.returncode != 0 or not root:
        sys.exit(f"checkdocs: cannot resolve GOROOT\n{done.stderr}")
    binary = pathlib.Path(root) / "bin" / ("go.exe" if os.name == "nt" else "go")
    if not binary.is_file():
        sys.exit(f"checkdocs: {binary} is not there")
    return str(binary)


GO = go_binary()


def blocks(document: str) -> list[tuple[int, str]]:
    text = (REPO / document).read_text(**UTF8)
    return [(i, m.group(1)) for i, m in enumerate(FENCE.finditer(text), 1)]


def scaffold(tmp: pathlib.Path) -> None:
    # Forward slashes, which go.mod takes on every platform, and quoted,
    # because it splits an unquoted path on the first space. A Windows clone
    # lives under C:/Users/First Last often enough.
    repo = REPO.as_posix()
    (tmp / "go.mod").write_text(
        "module checkdocs\n\ngo 1.24\n\n"
        f'replace github.com/mpyw/go-skill-embed => "{repo}"\n'
        f'replace github.com/mpyw/go-skill-embed/skillcobra => "{repo}/skillcobra"\n'
        f'replace github.com/mpyw/go-skill-embed/skillurfavev2 => "{repo}/skillurfavev2"\n'
        f'replace github.com/mpyw/go-skill-embed/skillurfavev3 => "{repo}/skillurfavev3"\n',
        **UTF8,
    )
    (tmp / "deps").mkdir()
    (tmp / "deps" / "deps.go").write_text(DEPS, **UTF8)
    (tmp / "block" / "skills" / "demo").mkdir(parents=True)
    (tmp / "block" / "skills" / "demo" / "SKILL.md").write_text("---\nname: demo\n---\n", **UTF8)
    done = subprocess.run([GO, "mod", "tidy"], cwd=tmp, capture_output=True, text=True)
    if done.returncode != 0:
        sys.exit(f"checkdocs: cannot resolve the modules\n{done.stderr}")


def wrap(block: str) -> str:
    """Give the block whatever it needs around it to be a package."""
    if re.search(r"^package ", block, re.M):
        return block

    # A block that declares things, including the quick start, brings its own
    # embed directive and sometimes its own main.
    if re.match(r"^(//go:embed|func|var|const|type)\b", block.lstrip()):
        source = HEADER
        if not re.search(r"^var skills\b", block, re.M):
            source += AUX
        source += "\n" + block
        if not re.search(r"^func main\(", block, re.M):
            source += "\nfunc main() {}\n"
        return source

    # A statement block. Anything it declares is blank-used, so that a snippet
    # written for a reader does not have to satisfy the compiler as well.
    names = []
    for match in DECLARED.finditer(block):
        names.extend(n.strip() for n in match.group(1).split(",") if n.strip() != "_")
    uses = "".join(f"\t_ = {name}\n" for name in names)
    return HEADER + AUX + "\nfunc main() {\n" + block + uses + "}\n"


def check(tmp: pathlib.Path, document: str, index: int, block: str) -> bool:
    source = wrap(block)
    where = f"{document} block {index}"
    if source is None:
        print(f"SKIP {where}: cannot classify\n{block}", file=sys.stderr)
        return False

    path = tmp / "block" / "check.go"
    path.write_text(source, **UTF8)
    # os.devnull is NUL on Windows and /dev/null elsewhere. The build output is
    # discarded either way.
    done = subprocess.run([GO, "build", "-o", os.devnull, "./block"], cwd=tmp, capture_output=True, text=True)
    path.unlink()
    if done.returncode != 0:
        print(f"FAIL {where}\n{block}\n{done.stderr}", file=sys.stderr)
        return False
    return True


def main() -> None:
    tmp = pathlib.Path(tempfile.mkdtemp())
    try:
        scaffold(tmp)
        failed = 0
        total = 0
        for document in DOCUMENTS:
            for index, block in blocks(document):
                total += 1
                if not check(tmp, document, index, block):
                    failed += 1
        print(f"checkdocs: {total - failed}/{total} Go blocks compile")
        sys.exit(1 if failed else 0)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    main()
