import { IncomingMessage, ServerResponse } from "http";

import { sendJson } from "../http/json_response";
import { DeviceContextRepository } from "../repositories/device_context_repository";

export class DeviceContextController {
  private readonly repository: DeviceContextRepository;

  constructor(repository: DeviceContextRepository) {
    this.repository = repository;
  }

  async getDeviceContext(
    _req: IncomingMessage,
    res: ServerResponse
  ): Promise<void> {
    try {
      const payload = await this.repository.loadContext();
      res.writeHead(200, {
        "Content-Type": "application/json",
        "Cache-Control": "no-store"
      });
      res.end(payload);
    } catch (error) {
      sendJson(res, 500, { error: "failed to load device context" });
    }
  }
}
