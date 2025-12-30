import { IncomingMessage, ServerResponse } from "http";

import { sendJson } from "../http/json_response";
import { BadRequestError, readJson } from "../http/read_json";
import {
  ChatRequest,
  LLMResponseRepository
} from "../repositories/llm_response_repository";

export class LLMController {
  private readonly repository: LLMResponseRepository;

  constructor(repository: LLMResponseRepository) {
    this.repository = repository;
  }

  async createChatCompletion(
    req: IncomingMessage,
    res: ServerResponse
  ): Promise<void> {
    try {
      const payload = await readJson<ChatRequest>(req);
      const response = await this.repository.loadResponse(payload);
      sendJson(res, 200, response);
    } catch (error) {
      if (error instanceof SyntaxError || error instanceof BadRequestError) {
        sendJson(res, 400, { error: error.message });
        return;
      }
      sendJson(res, 500, { error: "failed to build LLM response" });
    }
  }
}
