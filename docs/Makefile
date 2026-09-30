# Documentation site build (Sphinx, managed with uv). See ADR-0022.
#
#   make html      - build the site into _build/html
#   make serve     - live-reloading preview on http://127.0.0.1:8000
#   make check     - strict build (warnings are errors) plus link check
#   make clean     - remove _build

UV        ?= uv
SPHINX    := $(UV) run --frozen sphinx-build
SOURCEDIR := .
BUILDDIR  := _build

.PHONY: html serve check linkcheck clean

html:
	$(SPHINX) -b html $(SOURCEDIR) $(BUILDDIR)/html

serve:
	$(UV) run --frozen sphinx-autobuild --ignore "*.md" $(SOURCEDIR) $(BUILDDIR)/html

check:
	$(SPHINX) -b html -W --keep-going -n $(SOURCEDIR) $(BUILDDIR)/html
	$(MAKE) linkcheck

linkcheck:
	$(SPHINX) -b linkcheck -q $(SOURCEDIR) $(BUILDDIR)/linkcheck

clean:
	rm -rf $(BUILDDIR)
