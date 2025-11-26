---
applyTo: '**/*'
---

## Code Review Guidelines

When performing code reviews for pull requests in the Radius repository, follow these comprehensive steps.

**Important**: When reviewing code in specific languages or technologies, refer to the corresponding instruction files in `.github/instructions/*.instructions.md` for language-specific best practices, conventions, and review criteria. These files contain detailed guidelines for Go, Shell scripts, GitHub Workflows, and other technologies used in this project.

**Error Handling:**
- If files are too large to analyze completely, focus on the most critical changes and note it in the review.
- If unable to access certain files, note this limitation in the review.

### Code Review Principles

- **Be concise and direct**: Use short, imperative statements rather than long paragraphs
- **Focus on actionable feedback**: Avoid vague directives like "be more accurate" or "identify all issues"
- **Structure matters**: Use bullet points and clear headings for organization
- **Show examples**: Demonstrate concepts with sample code when clarification is needed
- **Be specific**: Provide exact file paths, line numbers, and clear issue descriptions
- **Avoid generic praise**: Remove purely complimentary comments; focus on improvements needed

### Before Starting a Review

1. Use the active pull request context to understand the PR background
2. Review the project's contributing guidelines if available
3. Check for related GitHub issues referenced in the PR description
4. Look at any previous discussions and comments on the PR

### Step 1: Analyze the Changes

Conduct a detailed file-by-file analysis of the PR:

- **PR Summary**: Understand the overall purpose and scope of the changes
- **File-by-file analysis**: For each changed file, document:
  - The file's purpose and role in the codebase
  - Specific changes made
  - Impact assessment of those changes

Consider both the PR author's description and the actual code changes when analyzing.

### Step 2: Provide Review Feedback

Review the analyzed changes and provide constructive, actionable feedback:

- **Focus**: Look for issues, bugs, and non-idiomatic language usage
- **Avoid**: Purely complimentary comments or unnecessary summaries
- **Format**: Keep comments concise and focused on changes that need to be made
- **Structure**: Organize feedback by file with specific line references

#### General Code Quality Criteria

As a world-class programming expert and good teammate, evaluate:

- **Idiomatic usage**: Does the code follow language-specific best practices?
- **Code quality**: Is the code maintainable and well-structured?
- **Readability**: Is the code clear and easy to understand?
- **Simplicity**: Is the code as simple as possible? Avoid unnecessary complexity
- **Performance**: Are there potential performance issues?
- **Bugs**: Are there any potential bugs or edge cases not handled?

#### Unit Test Review Criteria

When reviewing tests, specifically look for:

- **Parallel execution**: Are tests using parallel execution where possible?
- **Test consolidation**: Flag copy/paste tests that could be consolidated with parameters
- **Test clarity**: Are test cases clear and concise?
- **Mocking/stubbing**: Proper use of test doubles
- **Organization**: Proper structure of test files and functions
- **Assertions**: Adequate assertions to verify expected behavior
- **Setup/teardown**: Proper handling of test lifecycle
- **Naming conventions**: Clear, descriptive test names
- **Framework usage**: Appropriate use of test frameworks and libraries
- **Helper functions**: Good reuse to avoid duplication
- **Coverage**: Adequate coverage of edge cases and error conditions

### Step 3: Validate Your Review

Act as a critic of your own review to ensure:

- **Accuracy**: File names, paths, and line numbers are correct
- **Clarity**: Comments are clear, concise, and actionable
- **Value**: Remove any comments that are purely complimentary
- **Correctness**: Fix any discrepancies found during validation

### Review Output Format

Structure your review comments as:

```
path/to/file.ext
    Line X: Specific issue description
    Line Y: Suggestion for improvement
```

Include an overall PR assessment summarizing the key findings and recommendations.

### Best Practices for Effective Reviews

1. **Clear titles**: Use descriptive headings for different review sections
2. **Prefer specific over general**: "Use `const` instead of `let` on line 42" is better than "improve variable declarations"
3. **Provide rationale**: Explain why a change is needed, not just what to change
4. **Code examples**: Show correct and incorrect patterns when clarifying complex issues
5. **Language context**: Consider language-specific idioms and best practices
6. **Consistency**: Ensure feedback aligns with existing codebase patterns
7. **Readability focus**: Prioritize code clarity and maintainability
