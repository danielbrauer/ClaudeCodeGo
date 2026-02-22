package tui

import "fmt"

// buildReviewPrompt constructs the prompt sent to the agentic loop for /review.
// If prNumber is non-empty, it targets a specific PR; otherwise it lists open PRs.
func buildReviewPrompt(prNumber string) string {
	return fmt.Sprintf(`You are an expert code reviewer. Follow these steps:

1. If no PR number is provided in the args, run %[1]cgh pr list%[1]c to show open PRs
2. If a PR number is provided, run %[1]cgh pr view <number>%[1]c to get PR details
3. Run %[1]cgh pr diff <number>%[1]c to get the diff
4. Analyze the changes and provide a thorough code review that includes:
   - Overview of what the PR does
   - Analysis of code quality and style
   - Specific suggestions for improvements
   - Any potential issues or risks

Keep your review concise but thorough. Focus on:
- Code correctness
- Following project conventions
- Performance implications
- Test coverage
- Security considerations

Format your review with clear sections and bullet points.

PR number: %s`, '`', prNumber)
}
