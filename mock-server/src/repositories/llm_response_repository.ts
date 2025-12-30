import { readFile } from "fs/promises";

export interface ChatMessage {
  role: string;
  content: string;
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
}

export interface ChatRequest {
  model?: string;
  messages: ChatMessage[];
  tools?: unknown[];
}

export interface ChatResponseChoice {
  message: ChatMessage;
  tool_calls?: ToolCall[];
}

export interface ChatResponse {
  choices: ChatResponseChoice[];
}

export interface LLMResponseRepository {
  loadResponse(request: ChatRequest): Promise<ChatResponse>;
}

export class FileLLMResponseRepository implements LLMResponseRepository {
  private readonly path: string;

  constructor(path: string) {
    this.path = path;
  }

  async loadResponse(_request: ChatRequest): Promise<ChatResponse> {
    const payload = await readFile(this.path, "utf8");
    return JSON.parse(payload) as ChatResponse;
  }
}

export class EchoLLMResponseRepository implements LLMResponseRepository {
  async loadResponse(request: ChatRequest): Promise<ChatResponse> {
    const message = this.pickLastUserMessage(request);
    const content = message ? `Echo: ${message.content}` : "Mock response";

    return {
      choices: [
        {
          message: {
            role: "assistant",
            content
          }
        }
      ]
    };
  }

  private pickLastUserMessage(request: ChatRequest): ChatMessage | undefined {
    if (!request.messages || request.messages.length === 0) {
      return undefined;
    }

    for (let i = request.messages.length - 1; i >= 0; i -= 1) {
      if (request.messages[i].role === "user") {
        return request.messages[i];
      }
    }

    return request.messages[request.messages.length - 1];
  }
}
