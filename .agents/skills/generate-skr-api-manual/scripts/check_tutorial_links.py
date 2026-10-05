#!/usr/bin/env python3

"""Check external links in Markdown API tutorials."""

from __future__ import annotations

import argparse
import concurrent.futures
import dataclasses
import pathlib
import re
import sys
import time
import urllib.error
import urllib.request
from urllib.parse import unquote, urlsplit


LINK_PATTERN = re.compile(
    r"(?<!!)\[[^\]]*\]\((?P<target><[^>]+>|[^)\s]+)(?:\s+[^)]*)?\)"
    r"|<(?P<autolink>https?://[^>\s]+)>"
    r"|!\[[^\]]*\]\((?P<image><[^>]+>|[^)\s]+)(?:\s+[^)]*)?\)"
)
URL_PATTERN = re.compile(r"https?://[^\s<>()\[\]{}\"']+")
TRAILING_PUNCTUATION = ".,;:!?`"
DEFAULT_TUTORIAL_PATTERN = "*.md"
USER_AGENT = "skr-tutorial-link-checker/1.0"


@dataclasses.dataclass(frozen=True)
class Occurrence:
    path: pathlib.Path
    line: int


@dataclasses.dataclass(frozen=True)
class LocalLink:
    path: pathlib.Path
    line: int
    target: str


@dataclasses.dataclass(frozen=True)
class LinkResult:
    url: str
    status: int | None
    final_url: str | None
    error: str | None

    @property
    def ok(self) -> bool:
        return self.error is None and self.status is not None and 200 <= self.status < 400


def markdown_lines(path: pathlib.Path) -> list[str]:
    lines: list[str] = []
    fence_char: str | None = None
    fence_length = 0
    for line in path.read_text(encoding="utf-8").splitlines():
        fence = re.match(r"^\s*(`{3,}|~{3,})", line)
        if fence:
            marker = fence.group(1)
            if fence_char is None:
                fence_char = marker[0]
                fence_length = len(marker)
            elif marker[0] == fence_char and len(marker) >= fence_length:
                fence_char = None
            lines.append("")
            continue
        lines.append("" if fence_char else line)
    return lines


def extract_links(path: pathlib.Path) -> tuple[dict[str, list[Occurrence]], list[LocalLink]]:
    external: dict[str, list[Occurrence]] = {}
    local: list[LocalLink] = []
    for line_number, line in enumerate(markdown_lines(path), start=1):
        line = re.sub(r"`[^`]*`", "", line)
        for match in LINK_PATTERN.finditer(line):
            target = next(
                (value for value in match.groupdict().values() if value is not None),
                "",
            ).strip("<>")
            if target.startswith(("http://", "https://")):
                url = target.rstrip(TRAILING_PUNCTUATION)
                external.setdefault(url, []).append(Occurrence(path, line_number))
            elif target and not target.startswith(("mailto:", "#")):
                local.append(LocalLink(path, line_number, target))
        for match in URL_PATTERN.finditer(line):
            url = match.group(0).rstrip(TRAILING_PUNCTUATION)
            if url not in external:
                external.setdefault(url, []).append(Occurrence(path, line_number))
    return external, local


def local_link_exists(link: LocalLink) -> bool:
    target = urlsplit(link.target)
    if target.scheme or target.netloc:
        return True
    relative_path = unquote(target.path)
    resolved = (link.path.parent / relative_path).resolve() if relative_path else link.path.resolve()
    return resolved.is_file()


def check_link(url: str, timeout: float, retries: int) -> LinkResult:
    request = urllib.request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,application/xhtml+xml,*/*;q=0.8",
        },
        method="GET",
    )
    for attempt in range(retries + 1):
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                response.read(1)
                return LinkResult(url, response.status, response.geturl(), None)
        except urllib.error.HTTPError as error:
            status = error.code
            final_url = error.geturl()
            reason = error.reason
            error.close()
            if status < 500 or attempt == retries:
                return LinkResult(url, status, final_url, f"HTTP {status} {reason}")
        except (urllib.error.URLError, TimeoutError) as error:
            if attempt == retries:
                reason = error.reason if isinstance(error, urllib.error.URLError) else error
                return LinkResult(url, None, None, str(reason))
        time.sleep(0.5 * (attempt + 1))
    raise AssertionError("unreachable")


def default_paths(
    root: pathlib.Path = pathlib.Path("docs/manual/tutorials"),
) -> list[pathlib.Path]:
    return sorted(root.rglob(DEFAULT_TUTORIAL_PATTERN))


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Check links in Markdown tutorials under docs/manual/tutorials.",
    )
    parser.add_argument("paths", nargs="*", type=pathlib.Path, help="Markdown files to check")
    parser.add_argument("--timeout", type=float, default=15.0, help="Timeout per request in seconds")
    parser.add_argument("--retries", type=int, default=1, help="Retries for server and network errors")
    parser.add_argument("--jobs", type=int, default=4, help="Maximum concurrent requests")
    args = parser.parse_args(argv)
    if args.timeout <= 0:
        parser.error("--timeout must be greater than zero")
    if args.retries < 0:
        parser.error("--retries must not be negative")
    if args.jobs <= 0:
        parser.error("--jobs must be greater than zero")
    return args


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv if argv is not None else sys.argv[1:])
    paths = args.paths or default_paths()
    missing = [path for path in paths if not path.is_file()]
    if missing:
        for path in missing:
            print(f"ERROR: file not found: {path}", file=sys.stderr)
        return 2
    if not paths:
        print("ERROR: no Markdown tutorials found under docs/manual/tutorials", file=sys.stderr)
        return 2

    links: dict[str, list[Occurrence]] = {}
    local_links: list[LocalLink] = []
    for path in paths:
        external, local = extract_links(path)
        for url, occurrences in external.items():
            links.setdefault(url, []).extend(occurrences)
        local_links.extend(local)
    missing_local = [link for link in local_links if not local_link_exists(link)]
    if missing_local:
        for link in missing_local:
            print(
                f"BROKEN: local link {link.target}: {link.path}:{link.line}",
                file=sys.stderr,
            )
        return 1
    if not links:
        print("ERROR: no HTTP(S) links found", file=sys.stderr)
        return 2

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as executor:
        futures = {
            executor.submit(check_link, url, args.timeout, args.retries): url
            for url in links
        }
        results = [future.result() for future in concurrent.futures.as_completed(futures)]

    failures = 0
    for result in sorted(results, key=lambda item: item.url):
        if result.ok:
            redirect = f" -> {result.final_url}" if result.final_url != result.url else ""
            print(f"OK {result.status}: {result.url}{redirect}")
            continue
        failures += 1
        print(f"BROKEN: {result.url}: {result.error}", file=sys.stderr)
        for occurrence in links[result.url]:
            print(f"  {occurrence.path}:{occurrence.line}", file=sys.stderr)
    print(f"Checked {len(links)} unique links in {len(paths)} files; {failures} broken.")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
