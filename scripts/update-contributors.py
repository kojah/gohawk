#!/usr/bin/env python3
"""Render GitHub's commit contributors as a grid of round avatars in one SVG."""

import argparse
import base64
import html
import json
from pathlib import Path
import subprocess
import urllib.request

SIZE = 40
GAP = 6
COLUMNS = 18


def contributors(pages):
    # GitHub paginates this endpoint; retain its contribution-count ordering.
    people = [person for page in pages for person in page if person["type"] == "User"]
    if not people:
        raise ValueError("GitHub returned no human contributors; leaving the image unchanged")
    return people


def avatar_data_uri(url):
    # GitHub renders README SVGs as images, which cannot load external resources,
    # so each avatar is embedded. Fetch at twice the display size for HiDPI screens.
    separator = "&" if "?" in url else "?"
    with urllib.request.urlopen(f"{url}{separator}s={2 * SIZE}", timeout=30) as response:
        kind = response.headers.get_content_type()
        data = base64.b64encode(response.read()).decode("ascii")
    return f"data:{kind};base64,{data}"


def render_svg(people, avatars):
    columns = min(len(people), COLUMNS)
    rows = -(-len(people) // columns)
    width = columns * (SIZE + GAP) - GAP
    height = rows * (SIZE + GAP) - GAP
    lines = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" '
        f'viewBox="0 0 {width} {height}" role="img" aria-label="gohawk contributors">',
        '<defs><clipPath id="avatar" clipPathUnits="objectBoundingBox">'
        '<circle cx="0.5" cy="0.5" r="0.5"/></clipPath></defs>',
    ]
    for index, (person, avatar) in enumerate(zip(people, avatars)):
        x = index % columns * (SIZE + GAP)
        y = index // columns * (SIZE + GAP)
        login = html.escape(person["login"], quote=True)
        lines.append(
            f'<image x="{x}" y="{y}" width="{SIZE}" height="{SIZE}" clip-path="url(#avatar)" '
            f'href="{avatar}"><title>@{login}</title></image>'
        )
    lines.append("</svg>")
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository", required=True, help="GitHub owner/repository")
    parser.add_argument("--output", type=Path, default=Path(".github/contributors.svg"))
    args = parser.parse_args()
    response = subprocess.check_output(
        ["gh", "api", "--paginate", "--slurp", f"repos/{args.repository}/contributors"],
        text=True,
    )
    people = contributors(json.loads(response))
    svg = render_svg(people, [avatar_data_uri(person["avatar_url"]) for person in people])
    original = args.output.read_text(encoding="utf-8") if args.output.exists() else ""
    if svg != original:
        args.output.write_text(svg, encoding="utf-8")
        print("Updated contributor avatars")
    else:
        print("Contributor avatars are already current")


if __name__ == "__main__":
    main()
