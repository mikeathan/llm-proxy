import type { AssistantRole } from '../types/assistant';

/**
 * Domain logic for assistant roles and states.
 */


/**
 * Checks if a role is the system role.
 */
export const isSystemRole = (role: string | AssistantRole): boolean => {
  return role === 'system';
};

/**
 * Gets the display label for a message role.
 */
export const getRoleLabel = (role: string | AssistantRole): string => {
  if (isSystemRole(role)) return 'System:';
  if (role === 'user') return 'You:';
  return 'Assistant:';
};


/**
 * Determines CSS class for a role.
 */
export const getRoleClass = (role: string | AssistantRole): string => {
  return isSystemRole(role) ? 'role-error' : 'role-assistant';
};

/**
 * Determines CSS class for a message container based on role.
 */
export const getMessageClass = (role: string | AssistantRole): string => {
  return isSystemRole(role) ? 'system-error-msg' : '';
};

/**
 * Determines CSS class for message content based on role.
 */
export const getContentClass = (role: string | AssistantRole): string => {
  return isSystemRole(role) ? 'content-error' : '';
};
