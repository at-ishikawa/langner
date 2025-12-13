## Update OpenAI's prompt

Please update prompts of OpenAI's clients.

This is the current test errors:
$ARGUMENTS

### How to update a prompt

Now, you can run an integration test by `go test -v ./internal/inference/openai -tags integration`.
Based on this integration, test result,

1. Evaluate the scores of precision
2. Analyze each of test targets in `internal/inference/openai/client_integration_data_test.go`
3. Update the prompt message following prompt rules written below. Ask openai-prompt-optimizer agent to update the prompt.
4. After updating the prompt, run an integration test again and make sure the score improved. If it's not, revert your change and give me what prompt is better to improve the prompt by claude code.


### Prompt rules

1. To get the correct answers is the most important things. At the same time, make the prompt as simple as possible
2. MUST NOT INCLUDE specific words or phrases in a prompt from test cases. Only general words or phrases can be included.
