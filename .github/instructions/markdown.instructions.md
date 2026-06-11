---
description: 'Markdown formatting best practices based on markdownlint rules'
applyTo: '**/*.md'
---

# Markdown Guidelines

Instructions for writing clean, consistent, and accessible Markdown documents based on [markdownlint rules](https://github.com/DavidAnson/markdownlint/blob/main/doc/Rules.md).

## Headings

- **MD001**: Heading levels should only increment by one level at a time (e.g., don't skip from `#` to `###`)
- **MD003**: Use a consistent heading style throughout the document (atx `#` style recommended)
- **MD018**: Include a space after the hash character in atx-style headings (`# Heading` not `#Heading`)
- **MD019**: Use only one space after hash characters in atx-style headings
- **MD020**: Include spaces inside hashes on closed atx-style headings if using that style
- **MD021**: Use only one space inside hashes on closed atx-style headings
- **MD022**: Surround headings with blank lines (one before and one after)
- **MD023**: Headings must start at the beginning of the line without indentation
- **MD025**: Use only one top-level heading (`# Title`) per document (front matter title is not counted toward this rule)
- **MD026**: Avoid trailing punctuation in headings (periods, semicolons, colons, exclamation marks, and full-width equivalents; question marks are allowed by default)
- **MD043**: Follow required heading structure when enforced by project conventions

> **Disabled in this repository:** MD024 (duplicate heading content), MD036 (emphasis used as a heading), and MD041 (first line must be a top-level heading) are turned off in the markdownlint configuration and are not enforced.

## Lists

- **MD004**: Use dashes (`-`) for all unordered list items
- **MD005**: Use consistent indentation for list items at the same level
- **MD007**: Indent nested unordered list items by 2 spaces
- **MD029**: Use either all `1.` prefixes or an ordered (`1.`, `2.`, `3.`) sequence for ordered list items
- **MD030**: Use one space after list markers (`-`, `*`, `1.`)
- **MD032**: Surround lists with blank lines

## Whitespace and Line Length

- **No hard wrapping**: Do not insert manual line breaks within paragraphs or sentences. Let each paragraph be a single long line. Hard-wrapped prose creates noisy diffs, breaks reflowing in editors, and is not required by markdownlint. This applies to all prose — paragraphs, list item text, blockquote content, and image/link alt text. Code blocks and tables are excluded.
- **MD013**: Line length is not enforced (the rule is disabled), consistent with the no-hard-wrapping guidance above.
- **MD009**: Remove trailing spaces at the end of lines (except for intentional line breaks using 2 spaces)
- **MD010**: Use spaces instead of hard tab characters for indentation
- **MD012**: Avoid multiple consecutive blank lines
- **MD047**: End files with a single newline character

## Code Blocks

- **MD014**: Don't prefix every command with `$` in code blocks unless showing output
- **MD031**: Surround fenced code blocks with blank lines
- **MD038**: Avoid unnecessary leading/trailing spaces inside code span elements
- **MD040**: Specify a language for fenced code blocks for syntax highlighting
- **MD046**: Use fenced code blocks (not indented)
- **MD048**: Use a consistent code fence style (backticks recommended over tildes)

## Links and Images

- **MD011**: Use correct link syntax `[text](url)` not reversed `(text)[url]`
- **MD034**: Use angle brackets around bare URLs (`<https://example.com>`) or proper link syntax
- **MD039**: Avoid spaces inside link text brackets
- **MD042**: Don't use empty links with no destination
- **MD045**: Include alternate text (alt text) for all images for accessibility
- **MD051**: Ensure link fragments reference valid heading anchors within the document
- **MD052**: Ensure reference links use labels that are defined in the document
- **MD053**: Remove unused link and image reference definitions
- **MD054**: Use consistent link and image styles throughout the document
- **MD059**: Use descriptive link text instead of generic phrases like "click here", "here", "link", or "more"

## Blockquotes

- **MD027**: Use only one space after the blockquote symbol (`>`)
- **MD028**: Avoid blank lines inside blockquotes (use `>` on empty lines to continue the quote)

## Emphasis and Formatting

- **MD037**: Don't include spaces inside emphasis markers (`**bold**` not `** bold **`)
- **MD049**: Use a consistent emphasis style (asterisks `*italic*` recommended)
- **MD050**: Use a consistent strong/bold style (asterisks `**bold**` recommended)

## Tables

- **MD055**: Use consistent leading and trailing pipe characters in table rows
- **MD056**: Ensure all table rows have the same number of columns
- **MD058**: Surround tables with blank lines
- **MD060**: Use consistent table column alignment and formatting style

## Horizontal Rules

- **MD035**: Use a consistent horizontal rule style throughout the document (e.g., `---`)

## HTML

- **MD033**: Avoid inline HTML; only the `<br>` and `<pre>` elements are permitted

## Spelling and Capitalization

- **MD044**: Use correct capitalization for proper names and project terms

## Validation

- After creating or modifying Markdown files, review them against the markdownlint rules above to catch formatting violations.
