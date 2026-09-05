#!/usr/bin/env python3
"""Validate the Codex marketplace and its local plugin manifests."""

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MARKETPLACE = ROOT / ".agents" / "plugins" / "marketplace.json"
EXPECTED_PLUGINS = {
    "revdiff": {
        "source": "./plugins/codex",
        "files": (
            "skills/revdiff/SKILL.md",
            "skills/revdiff-plan/SKILL.md",
        ),
    },
    "revdiff-planning": {
        "source": "./plugins/revdiff-planning",
        "files": ("hooks/codex-hooks.json",),
    },
}


def load_json(path: Path) -> dict:
    with path.open(encoding="utf-8") as stream:
        return json.load(stream)


def main() -> None:
    if not __debug__:
        raise RuntimeError("assertions must be enabled for marketplace validation")

    marketplace = load_json(MARKETPLACE)
    claude_marketplace = load_json(ROOT / ".claude-plugin" / "marketplace.json")
    claude_versions = {
        plugin["name"]: plugin["version"] for plugin in claude_marketplace["plugins"]
    }
    plugins = {plugin["name"]: plugin for plugin in marketplace["plugins"]}
    assert set(plugins) == set(EXPECTED_PLUGINS), "unexpected Codex plugin set"

    for name, expected in EXPECTED_PLUGINS.items():
        plugin = plugins[name]
        source = plugin["source"]
        assert source["source"] == "local", f"{plugin['name']}: source must be local"
        assert source["path"] == expected["source"], (
            f"{name}: source is {source['path']}, expected {expected['source']}"
        )

        plugin_root = (ROOT / source["path"]).resolve()
        assert plugin_root.is_relative_to(ROOT), f"{plugin['name']}: source escapes repo"

        manifest_path = plugin_root / ".codex-plugin" / "plugin.json"
        manifest = load_json(manifest_path)
        assert manifest["name"] == plugin["name"], (
            f"{plugin['name']}: manifest name is {manifest['name']}"
        )
        assert manifest["version"] == claude_versions[plugin["name"]], (
            f"{plugin['name']}: Codex and Claude marketplace versions differ"
        )

        for field in ("skills", "hooks"):
            if field not in manifest:
                continue
            component = (plugin_root / manifest[field]).resolve()
            assert component.is_relative_to(plugin_root), (
                f"{plugin['name']}: {field} path escapes plugin"
            )
            assert component.exists(), f"{plugin['name']}: missing {field} path"

        for relative_path in expected["files"]:
            assert (plugin_root / relative_path).is_file(), (
                f"{plugin['name']}: missing packaged file {relative_path}"
            )

    print("Codex marketplace manifests are valid")


if __name__ == "__main__":
    main()
