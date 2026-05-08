package core

const MaxDiscordMessageLen = 2000

// ChunkMessage splits content into chunks of at most maxLen bytes.
func ChunkMessage(content string, maxLen int) []string {
	if content == "" {
		return nil
	}
	var chunks []string
	for len(content) > 0 {
		chunk := content
		if len(chunk) > maxLen {
			chunk = content[:maxLen]
			content = content[maxLen:]
		} else {
			content = ""
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}


