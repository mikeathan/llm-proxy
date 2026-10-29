import type { Template, TemplateMetadata } from "../../types/templates";
import { apiFetch } from "../httpClient";


export class TemplateService {
  /**
   * List all available templates (metadata only)
   */
  static async listTemplates(): Promise<TemplateMetadata[]> {
    const response = await apiFetch("/admin/api/templates");
    if (!response.ok) {
      throw new Error(`Failed to list templates: ${response.statusText}`);
    }
    return response.json();
  }

  /**
   * Get full details and content of a specific template
   */
  static async getTemplate(id: string): Promise<Template> {
    const response = await apiFetch(`/admin/api/templates/${id}`);
    if (!response.ok) {
      throw new Error(`Failed to get template: ${response.statusText}`);
    }
    return response.json();
  }
}
