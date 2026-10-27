#!/usr/bin/env python3
"""Refresh the README's contributor avatars from GitHub's commit contributors."""

import argparse
import html
import json
from pathlib import Path
import subprocess

START = "<!-- contributors:start -->"
END = "<!-- contributors:end -->"


def avatar_row(pages):
    # GitHub paginates this endpoint; retain its contribution-count ordering.
    contributors = [person for page in pages for person in page if person["type"] == "User"]
    if not contributors:
        raise ValueError("GitHub returned no human contributors; leaving the README unchanged")
    avatars = []
    for person in contributors:
        avatar = person["avatar_url"]
        separator = "&" if "?" in avatar else "?"
        image = html.escape(f"{avatar}{separator}s=64", quote=True)
        login = html.escape(person["login"], quote=True)
        profile = person["html_url"]
        avatars.append(
            f'[<img src="{image}" width="64" height="64" alt="@{login}">]({profile})'
        )
    return "\n".join(avatars)


def update_readme(text, row):
    if text.count(START) != 1 or text.count(END) != 1:
        raise ValueError("README must contain exactly one contributor marker pair")
    before, remainder = text.split(START, 1)
    _, after = remainder.split(END, 1)
    return f"{before}{START}\n{row}\n{END}{after}"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository", required=True, help="GitHub owner/repository")
    parser.add_argument("--readme", type=Path, default=Path("README.md"))
    args = parser.parse_args()
    response = subprocess.check_output(
        ["gh", "api", "--paginate", "--slurp", f"repos/{args.repository}/contributors"],
        text=True,
    )
    row = avatar_row(json.loads(response))
    original = args.readme.read_text(encoding="utf-8")
    updated = update_readme(original, row)
    if updated != original:
        args.readme.write_text(updated, encoding="utf-8")
        print("Updated contributor avatars")
    else:
        print("Contributor avatars are already current")


if __name__ == "__main__":
    main()
