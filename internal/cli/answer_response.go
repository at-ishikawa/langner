package cli

// AnswerResponse is a unified response structure for AI providers
type AnswerResponse struct {
	Correct    bool
	Expression string
	Meaning    string
	Context    string
}
