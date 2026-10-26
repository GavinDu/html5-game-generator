#!/usr/bin/env python3
"""Build a test-feedback.json artifact from a backend browser run.

Phase 2 of the testing framework (see Hephaestus docs/TESTING_FRAMEWORK.md):
the CI producer that turns a run into the artifact the AI coding feedback
loop consumes. Two report kinds, both Layer 0 (browser runs need no
device):

  playtest: the cmd/playtest worker report
      {base, browser, games: [{gameId, verdict: PASS|FAIL, checks}]}
      PASS -> pass, FAIL -> fail (class render, failed checks listed).
  kit: the playability-kit report, a list of
      {gameId, mutant, verdict: PLAYABLE|NEEDS_REVIEW|BROKEN, issues}
      PLAYABLE -> pass, BROKEN -> fail (class render). NEEDS_REVIEW ->
      fail (class unknown) with a detail that says so explicitly: the
      CI job tolerates NEEDS_REVIEW, but the game still needs review
      before it counts as passing, so the iteration brief must surface it.

Usage:
    tools/test-feedback/report-to-feedback.py --kind playtest \
        --report /tmp/playtest-report.json --out /tmp/test-feedback.json \
        --run-id <id> --commit <sha> [--browser chromium]
    tools/test-feedback/report-to-feedback.py --kind kit \
        --report /tmp/kit-out/report.json --out /tmp/test-feedback-kit.json \
        --run-id <id> --commit <sha>

Advisory by design, like the Hephaestus producers: a missing or
unparsable report yields a single skip case with a reason, never a
silent pass, and the script always exits 0. The CI steps that call it
must be if: always() and must not gate on it; the jobs' own verdicts
remain the gates.
"""

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def fail_case(prefix, detail):
    return {
        "id": f"{prefix}.producer",
        "verdict": "skip",
        "detail": detail,
    }


def playtest_cases(report):
    games = report.get("games")
    if not isinstance(games, list):
        return None
    cases = []
    for game in games:
        game_id = game.get("gameId", "unknown")
        verdict = game.get("verdict")
        if verdict == "PASS":
            cases.append({"id": f"playtest.{game_id}", "verdict": "pass"})
        elif verdict == "FAIL":
            failed = []
            for check in game.get("checks", []) or []:
                name = check.get("name") or check.get("id") or "check"
                if str(check.get("verdict", "")).upper() in ("FAIL", "FAILED"):
                    failed.append(name)
            detail = "Playtest gate failed."
            if failed:
                detail += f" Failed checks: {', '.join(failed)}."
            cases.append({
                "id": f"playtest.{game_id}",
                "verdict": "fail",
                "failure_class": "render",
                "detail": detail,
            })
        else:
            cases.append({
                "id": f"playtest.{game_id}",
                "verdict": "skip",
                "detail": f"Unknown playtest verdict {verdict!r}; not counted as a pass.",
            })
    return cases


def kit_cases(report):
    if not isinstance(report, list):
        return None
    cases = []
    for entry in report:
        game_id = entry.get("gameId", "unknown")
        mutant = entry.get("mutant")
        case_id = f"kit.{game_id}" + (f".mutant-{mutant}" if mutant else "")
        verdict = entry.get("verdict")
        issues = entry.get("issues", []) or []
        blockers = sum(1 for i in issues if i.get("severity") == "blocker")
        if verdict == "PLAYABLE":
            cases.append({"id": case_id, "verdict": "pass"})
        elif verdict == "BROKEN":
            cases.append({
                "id": case_id,
                "verdict": "fail",
                "failure_class": "render",
                "detail": f"Playability kit verdict BROKEN ({blockers} blocker issue(s), "
                          f"{len(issues)} issue(s) total).",
            })
        elif verdict == "NEEDS_REVIEW":
            cases.append({
                "id": case_id,
                "verdict": "fail",
                "failure_class": "unknown",
                "detail": "Playability kit verdict NEEDS_REVIEW: the CI job tolerates it, "
                          "but the game needs review before it counts as passing "
                          f"({len(issues)} issue(s)).",
            })
        else:
            cases.append({
                "id": case_id,
                "verdict": "skip",
                "detail": f"Unknown kit verdict {verdict!r}; not counted as a pass.",
            })
    return cases


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--kind", required=True, choices=["playtest", "kit"])
    parser.add_argument("--report", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--repo", default="GavinDu/html5-game-generator")
    parser.add_argument("--browser", default=None)
    args = parser.parse_args()

    cases = None
    try:
        report = json.loads(Path(args.report).read_text())
    except (OSError, ValueError) as exc:
        print(f"report-to-feedback: cannot read report: {exc}", file=sys.stderr)
        report = None
    if report is not None:
        cases = playtest_cases(report) if args.kind == "playtest" else kit_cases(report)
    if not cases:
        cases = [fail_case(args.kind, "No usable report found; "
                             "the producer could not determine the run outcome.")]

    summary = {
        "pass": sum(1 for c in cases if c["verdict"] == "pass"),
        "fail": sum(1 for c in cases if c["verdict"] == "fail"),
        "skip": sum(1 for c in cases if c["verdict"] == "skip"),
    }
    env = {"kind": "browser"}
    if args.browser:
        env["browser"] = args.browser
    artifact = {
        "schema_version": 1,
        "run_id": args.run_id,
        "created_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "commit": args.commit,
        "repo": args.repo,
        "layer": 0,
        "environment": env,
        "cases": cases,
        "summary": summary,
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(artifact, indent=2) + "\n")
    print(f"report-to-feedback: wrote {out} "
          f"({summary['pass']} pass, {summary['fail']} fail, {summary['skip']} skip)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
