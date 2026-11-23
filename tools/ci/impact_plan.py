"""Conservative impact planner. Input: one changed path per line, including old rename paths."""
import json
import sys

def plan(paths):
    suites = set()
    reasons = []
    for p in paths:
        if p.startswith("docs/") and p.endswith(".md"):
            selected = {"documentation"}
        elif p.startswith(".github/workflows/prune-"):
            selected = {"workflow-safety"}
        elif p.startswith(("Hephaestus/Player/", "internal/pipeline/", "tools/playtest/")):
            selected = {"unit", "integration", "runtime", "performance", "ui"}
        elif p.startswith("Hephaestus/Views/"):
            selected = {"unit", "ui", "performance"}
        elif p.startswith("internal/"):
            selected = {"unit", "integration", "performance", "runtime", "ui"}
        elif p.startswith("inventory/"):
            selected = {"integration", "runtime", "performance", "ui"}
        else:
            selected = {"full"}
        suites.update(selected)
        reasons.append({"path": p, "suites": sorted(selected)})
    if not paths or "full" in suites:
        suites.update({"full", "unit", "integration", "runtime", "performance", "ui", "workflow-safety", "documentation"})
    return {"suites": sorted(suites), "reasons": reasons, "advisory": True}

if __name__ == "__main__":
    print(json.dumps(plan([p for p in sys.stdin.read().splitlines() if p]), indent=2))
