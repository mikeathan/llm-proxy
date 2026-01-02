import { readFile } from "fs/promises";

export interface MetricsQueryRequest {
  device_id?: string;
  expose?: string;
  from?: number;
  to?: number;
  aggregate?: string;
  resolution?: string;
}

export interface MetricsQueryDeviceResponse {
  deviceId: string;
  value: unknown;
  timestamp?: number;
}

export interface MetricsQueryResponse {
  expose: string;
  from: number;
  to: number;
  values: MetricsQueryDeviceResponse[];
}

export interface MetricsQueryRepository {
  loadResponse(request: MetricsQueryRequest): Promise<MetricsQueryResponse>;
}

export class FileMetricsQueryRepository implements MetricsQueryRepository {
  private readonly path: string;

  constructor(path: string) {
    this.path = path;
  }

  async loadResponse(_request: MetricsQueryRequest): Promise<MetricsQueryResponse> {
    const payload = await readFile(this.path, "utf8");
    return JSON.parse(payload) as MetricsQueryResponse;
  }
}
