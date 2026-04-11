#!/usr/bin/env python3
"""Validate YAML frontmatter in all markdown files."""

import os
import sys

import yaml

failed = False
for root, dirs, files in os.walk("."):
    dirs[:] = [d for d in dirs if d not in (".git", "vendor")]
    for f in files:
        if not f.endswith(".md"):
            continue
        path = os.path.join(root, f)
        with open(path) as fh:
            content = fh.read()
        if not content.startswith("---\n"):
            continue
        end = content.find("\n---\n", 4)
        if end == -1:
            continue
        frontmatter = content[4:end]
        try:
            yaml.safe_load(frontmatter)
        except yaml.YAMLError as e:
            print(f"FAIL: {path}")
            print(f"  {e}")
            failed = True

if failed:
    sys.exit(1)
print("All markdown frontmatter is valid YAML")
