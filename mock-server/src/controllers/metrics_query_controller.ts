import { IncomingMessage, ServerResponse } from "http";

import { sendJson } from "../http/json_response";
import { BadRequestError, readJson } from "../http/read_json";
import {
  MetricsQueryRepository,
  MetricsQueryRequest
} from "../repositories/metrics_query_repository";

export class MetricsQueryController {
  private readonly repository: MetricsQueryRepository;

  constructor(repository: MetricsQueryRepository) {
    this.repository = repository;
  }

  async queryMetrics(
    req: IncomingMessage,
    res: ServerResponse
  ): Promise<void> {
    try {
      const payload = await readJson<MetricsQueryRequest>(req);
      const response = await this.repository.loadResponse(payload);
      sendJson(res, 200, response);
    } catch (error) {
      if (error instanceof SyntaxError || error instanceof BadRequestError) {
        sendJson(res, 400, { error: error.message });
        return;
      }
      sendJson(res, 500, { error: "failed to load metrics response" });
    }
  }
}
