## Update OpenAI's prompt

Please update prompts of OpenAI's clients.

This is the current test errors:
$ARGUMENTS

### Steps

1. Analyze each of test targets in `internal/inference/openai/client_integration_data_test.go`
2. Update the prompt message following prompt rules written below.

### Prompt rules

1. To get the correct answers is the most important things. At the same time, make the prompt as simple as possible
1. MUST NOT INCLUDE specific words or phrases in a prompt from test cases. Only general words or phrases can be included.
