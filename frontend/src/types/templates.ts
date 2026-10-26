import type { NetworkGrant } from './admin';

export interface Template {
  id: string;
  name: string;
  category: string;
  description: string;
  content: string;
  /** The automation settings the playbook wants; the form applies them and each can still be changed. */
  defaults?: TemplateDefaults;
}

export interface TemplateDefaults {
  profile?: string;
  network_grant?: NetworkGrant;
  journal?: boolean;
  /** A cron expression the form offers as the first schedule. */
  schedule?: string;
}

export interface TemplateMetadata {
  id: string;
  name: string;
  category: string;
  /** One line saying what the playbook is for. */
  description?: string;
  defaults?: TemplateDefaults;
}
