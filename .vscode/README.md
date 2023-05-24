# Configuration for VSCode development

The files in this folder configure VS Code's debugger and build settings. Not every project chooses to check in these files. On Radius our development setup is somewhat complex, and most of us use VS Code.

We made the decision to check in these files **and** add them to `.gitignore` so that we could:

- Easily share settings
- Document a smooth onramp for contributors
- Allow individuals to customize these files if they want

When committing changes to one of the files in the folder, you should use `git add -f <file>`.