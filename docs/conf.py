"""Sphinx configuration for the mcplake documentation site.

The Sphinx source root is ``docs/`` itself, so every page lives at the same
path in the repository as it does on the site (ADR-0022).
"""

import os

project = "mcplake"
author = "mcplake contributors"
copyright = "2026, mcplake contributors"

# CI sets DOCS_VERSION to "stable" (built from main) or "latest" (built from
# develop); a local build is "dev".
version = os.environ.get("DOCS_VERSION", "dev")
release = version

# Branch the "edit this page" links point at.
docs_branch = os.environ.get("DOCS_BRANCH", "develop")

extensions = [
    "sphinx.ext.extlinks",
    "sphinx.ext.todo",
    "sphinx_copybutton",
    "sphinx_design",
    "sphinxcontrib.mermaid",
    "sphinxcontrib.openapi",
]

# Only .rst is part of the site. Markdown files still under docs/ are either
# not migrated yet or internal working notes, and Sphinx ignores them.
source_suffix = {".rst": "restructuredtext"}
root_doc = "index"
exclude_patterns = [
    "_build",
    ".venv",
    "README.rst",
    "superpowers",
]

nitpicky = False
todo_include_todos = False

repo_url = "https://github.com/atsokha/mcplake"
extlinks = {
    "issue": (f"{repo_url}/issues/%s", "#%s"),
    "pr": (f"{repo_url}/pull/%s", "PR #%s"),
    "repo": (f"{repo_url}/blob/{docs_branch}/%s", "%s"),
}

rst_prolog = """
.. |project| replace:: mcplake
"""

# -- HTML output --------------------------------------------------------------

html_theme = "furo"
html_title = "mcplake"
html_static_path = ["_static"]
html_css_files = ["custom.css"]
html_copy_source = False
html_show_sourcelink = False
html_last_updated_fmt = "%Y-%m-%d"

html_theme_options = {
    "source_repository": f"{repo_url}/",
    "source_branch": docs_branch,
    "source_directory": "docs/",
    "navigation_with_keys": True,
    "light_css_variables": {
        "color-brand-primary": "#1f5fa8",
        "color-brand-content": "#1f5fa8",
    },
    "dark_css_variables": {
        "color-brand-primary": "#7fb2ee",
        "color-brand-content": "#7fb2ee",
    },
}

# -- Extensions ---------------------------------------------------------------

copybutton_prompt_text = r"\$ |>>> "
copybutton_prompt_is_regexp = True

mermaid_d3_zoom = False

# -- linkcheck ------------------------------------------------------------------

linkcheck_timeout = 15
linkcheck_retries = 2
linkcheck_ignore = [
    # The repository is private: GitHub answers 404 to anonymous requests.
    rf"{repo_url}.*",
    # Local endpoints used in examples.
    r"https?://localhost.*",
    r"https?://127\.0\.0\.1.*",
]
