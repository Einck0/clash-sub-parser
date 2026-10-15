#!/usr/bin/env python3
"""
CI diagnostic utility: parse `go test -json` output and emit GitHub Actions ::error annotations.
Runs with standard Python 3 and zero external dependencies.
"""

import json
import os
import sys
from typing import Dict, List, Optional, Tuple

MAX_FAILURES = 10
MAX_CHARS_PER_FAILURE = 4000


def escape_property(value: str) -> str:
    """Escapes special characters in GitHub Actions command property values."""
    return (
        value.replace("%", "%25")
        .replace("\r", "%0D")
        .replace("\n", "%0A")
        .replace(":", "%3A")
        .replace(",", "%2C")
    )


def escape_data(value: str) -> str:
    """Escapes special characters in GitHub Actions command message bodies."""
    return (
        value.replace("%", "%25")
        .replace("\r", "%0D")
        .replace("\n", "%0A")
    )


def format_bounded_output(raw_output: str, max_chars: int = MAX_CHARS_PER_FAILURE) -> str:
    """Truncates output to keep the failure-relevant tail if exceeding max_chars."""
    stripped = raw_output.strip()
    if not stripped:
        return "(no output captured)"
    if len(stripped) <= max_chars:
        return stripped
    prefix = f"... [truncated, showing last {max_chars} chars] ...\n"
    keep_len = max(0, max_chars - len(prefix))
    return prefix + stripped[-keep_len:]


def is_mere_parent_wrapper(
    pkg: str,
    test: str,
    outputs: List[str],
    all_failed_tests: List[Tuple[str, str]],
) -> bool:
    """
    Checks if a failing test is just a parent runner whose child subtests also failed,
    and whose own output contains no assertions besides RUN/FAIL banners.
    """
    prefix = test + "/"
    has_failed_children = any(
        p == pkg and t.startswith(prefix)
        for (p, t) in all_failed_tests
    )
    if not has_failed_children:
        return False

    content_lines = [
        line.strip()
        for line in outputs
        if line.strip()
        and not line.startswith("=== RUN")
        and not line.startswith("--- FAIL")
        and not line.startswith("=== CONT")
        and not line.startswith("=== PAUSE")
    ]
    return len(content_lines) == 0


def parse_and_annotate(
    json_path: str,
    exit_code: Optional[int] = None,
) -> int:
    if not os.path.isfile(json_path):
        if exit_code is not None and exit_code != 0:
            title = "Go Test Execution Failed"
            msg = f"go test exited with code {exit_code}, but output file {json_path} was not found."
            print(f"::error title={escape_property(title)}::{escape_data(msg)}")
            print(f"[FAIL] {title}: {msg}", file=sys.stderr)
        else:
            print(f"Info: {json_path} does not exist and exit code is 0.")
        return 0

    test_outputs: Dict[Tuple[str, str], List[str]] = {}
    package_outputs: Dict[str, List[str]] = {}
    failed_tests: List[Tuple[str, str]] = []
    failed_packages: List[str] = []
    unparsed_lines: List[str] = []

    with open(json_path, "r", encoding="utf-8", errors="replace") as f:
        for raw_line in f:
            line = raw_line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                unparsed_lines.append(line)
                continue

            action = event.get("Action")
            pkg = event.get("Package", "")
            test = event.get("Test", "")
            output = event.get("Output", "")

            if action == "output":
                if test:
                    test_outputs.setdefault((pkg, test), []).append(output)
                elif pkg:
                    package_outputs.setdefault(pkg, []).append(output)
            elif action == "fail":
                if test:
                    if (pkg, test) not in failed_tests:
                        failed_tests.append((pkg, test))
                elif pkg:
                    if pkg not in failed_packages:
                        failed_packages.append(pkg)

    failures: List[Dict[str, str]] = []

    # 1. Collect specific failed tests (filtering redundant parent wrappers)
    for (pkg, test) in failed_tests:
        outputs = test_outputs.get((pkg, test), [])
        if is_mere_parent_wrapper(pkg, test, outputs, failed_tests):
            continue
        formatted = format_bounded_output("".join(outputs))
        failures.append({
            "title": f"Go Test Failed: {pkg} {test}",
            "output": formatted,
        })

    # 2. Collect package-level failures (e.g. build/compile failures or panics outside test)
    for pkg in failed_packages:
        has_test_failures = any(p == pkg for (p, _) in failed_tests)
        if not has_test_failures:
            outputs = package_outputs.get(pkg, [])
            formatted = format_bounded_output("".join(outputs))
            failures.append({
                "title": f"Go Package Build/Execution Failed: {pkg}",
                "output": formatted,
            })

    # 3. Fallback if command exited non-zero but no fail actions detected in JSON
    if not failures and exit_code is not None and exit_code != 0:
        fallback_msg = "\n".join(unparsed_lines).strip()
        if not fallback_msg:
            # Check all package outputs
            all_pkg_outs = [
                "".join(outs) for outs in package_outputs.values() if "".join(outs).strip()
            ]
            fallback_msg = "\n".join(all_pkg_outs).strip()
        if not fallback_msg:
            fallback_msg = f"go test exited with code {exit_code} (no failure details parsed)."
        failures.append({
            "title": "Go Test Process Failed",
            "output": format_bounded_output(fallback_msg),
        })

    # If no failures and exit_code is 0 (or not failing), report success and emit nothing
    if not failures:
        print("All Go tests completed successfully (no failures detected).")
        return 0

    total_failures = len(failures)
    selected_failures = failures[:MAX_FAILURES]

    print("\n====================================================")
    print(f"Go Test Failure Diagnostics ({len(selected_failures)} of {total_failures} failure(s)):")
    for idx, item in enumerate(selected_failures, 1):
        print(f"\n[{idx}] {item['title']}:")
        print(item['output'])
    print("====================================================\n")

    for item in selected_failures:
        title = item["title"]
        msg = item["output"]
        print(f"::error title={escape_property(title)}::{escape_data(msg)}")

    if total_failures > MAX_FAILURES:
        overflow_count = total_failures - MAX_FAILURES
        overflow_msg = f"{overflow_count} additional failure(s) omitted due to truncation limit."
        print(f"::warning::{escape_data(overflow_msg)}")

    return 0


def main() -> None:
    json_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/go-test-output.json"
    exit_code: Optional[int] = None
    if len(sys.argv) > 2:
        try:
            exit_code = int(sys.argv[2])
        except ValueError:
            pass

    sys.exit(parse_and_annotate(json_file, exit_code))


if __name__ == "__main__":
    main()
