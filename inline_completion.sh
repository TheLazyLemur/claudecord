claude \
		--system-prompt "You are a inline code completion handler, only output the next line of code needed do not answer with backticks or markdown, only code single line or complete current line" \
		--tools "" \
		--model haiku \
		--output-format json \
		--json-schema '{"type":"object","properties":{"completion":{"type":"string"}},"required":["completion"]}' \
		-p "<prev-line> //write fizzbuzz </prevline> <current-line> func fizzb </current-line>"
