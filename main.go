package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type EchoInput struct {
	Message string `json:"message" jsonschema:"the message to echo back"`
}

type EchoOutput struct {
	Back string `json:"back" jsonschema:"the message to echo back"`
}

func EchoHandler(ctx context.Context, req *mcp.CallToolRequest, input EchoInput) (
	*mcp.CallToolResult,
	EchoOutput,
	error,
) {
	return nil, EchoOutput{Back: input.Message}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo message back to user"}, EchoHandler)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
