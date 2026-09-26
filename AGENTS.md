# Repository instructions

go-skill-embed installs embedded Agent Skills while preserving their source bytes and detecting local modifications. Keep README and code comments focused on current behavior. Design history, rejected approaches, platform cases, and accepted oddities belong in [implementation notes](design/implementation.md); read the relevant section before changing the installer or its tests.

Run `./test_all.sh` before claiming the full gate passes; it covers every module. Skill installation paths such as `.claude/skills` and `.agents/skills` are product behavior here, so keep them distinct from this repository's own agent instructions.
