package helper

import (
	"context"
	"github.com/gin-gonic/gin"
)

func CtxDone(c context.Context) bool {
	if ginC, ok := c.(*gin.Context); ok {
		select {
		case <-ginC.Request.Context().Done():
			return true
		default:
			return false
		}
	}

	return false
}

type Chunk struct {
	Start int64
	End   int64
	Index int
}

// 计算分片范围
func CalculateChunks(totalSize, chunkSize int64) []Chunk {
	var chunks []Chunk
	for i := int64(0); i < totalSize; i += chunkSize {
		end := i + chunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		chunks = append(chunks, Chunk{
			Start: i,
			End:   end,
			Index: len(chunks),
		})
	}
	return chunks
}
