# LaTeX implementation report

`main.tex` is the maintained source for the standalone OCI image-transfer implementation report.

Build it from this directory with:

```sh
pdflatex -interaction=nonstopmode -halt-on-error main.tex
pdflatex -interaction=nonstopmode -halt-on-error main.tex
```

Two passes resolve the table of contents and page references. The generated deliverable is `main.pdf`.

On Windows, `powershell -ExecutionPolicy Bypass -File build.ps1` uses `pdflatex`
when available. If TeX is not installed, it creates a readable PDF preview from
the same `main.tex` content using Microsoft Word and verifies that the exported
document contains pages and text. A real TeX installation is still recommended
for final typesetting.

Source line references describe the current feature branch. If implementation files change, update the affected ranges in `main.tex` and rebuild the PDF.
