import { resolve } from "path";

import { DeviceContextController } from "./controllers/device_context_controller";
import { LLMController } from "./controllers/llm_controller";
import { MetricsQueryController } from "./controllers/metrics_query_controller";
import { FileDeviceContextRepository } from "./repositories/device_context_repository";
import {
  EchoLLMResponseRepository,
  FileLLMResponseRepository
} from "./repositories/llm_response_repository";
import { FileMetricsQueryRepository } from "./repositories/metrics_query_repository";
import { Router } from "./router";
import { HttpServer } from "./server";

const port = Number.parseInt(process.env.PORT ?? "3001", 10);
const samplePath = resolve(__dirname, "..", "..", "docs", "samples", "device_context.json");
const llmSampleName =
  process.env.LLM_RESPONSE_SAMPLE ?? "llm_response_tool_call.json";
const llmResponsePath = resolve(
  __dirname,
  "..",
  "..",
  "docs",
  "samples",
  llmSampleName
);
const metricsResponsePath = resolve(
  __dirname,
  "..",
  "..",
  "docs",
  "samples",
  "metrics_query.json"
);

const repository = new FileDeviceContextRepository(samplePath);
const controller = new DeviceContextController(repository);
const llmRepository =
  (process.env.LLM_RESPONSE_MODE ?? "").toLowerCase() === "echo"
    ? new EchoLLMResponseRepository()
    : new FileLLMResponseRepository(llmResponsePath);
const llmController = new LLMController(llmRepository);
const metricsRepository = new FileMetricsQueryRepository(metricsResponsePath);
const metricsController = new MetricsQueryController(metricsRepository);
const router = new Router();

router.register("GET", "/api/context/devices", (req, res) =>
  controller.getDeviceContext(req, res)
);
router.register("POST", "/v1/chat/completions", (req, res) =>
  llmController.createChatCompletion(req, res)
);
router.register("POST", "/api/metrics/query", (req, res) =>
  metricsController.queryMetrics(req, res)
);

const server = new HttpServer(router);

server.listen(port, () => {
  console.log(`Mock server listening on http://localhost:${port}`);
  console.log("GET /api/context/devices");
  console.log("POST /v1/chat/completions");
  console.log("POST /api/metrics/query");
  console.log(`Device context sample: ${samplePath}`);
  console.log(
    llmRepository instanceof EchoLLMResponseRepository
      ? "LLM response mode: echo"
      : `LLM response sample: ${llmResponsePath}`
  );
});
