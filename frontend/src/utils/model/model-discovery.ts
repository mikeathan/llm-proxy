/** Formats bytes into a human-readable size (e.g. 4.2 GB) */
export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

/** Formats parameter count into a human-readable string (e.g. 8B, 70B) */
export function formatParameters(params: number): string {
  if (!params || params === 0) return '';
  if (params >= 1_000_000_000) {
    return (params / 1_000_000_000).toFixed(1).replace(/\.0$/, '') + 'B';
  }
  if (params >= 1_000_000) {
    return (params / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  }
  return params.toString();
}

const COLOR_PALETTES = [
  'bg-blue-500/20 text-blue-300 border-blue-500/20',
  'bg-teal-500/20 text-teal-300 border-teal-500/20',
  'bg-indigo-500/20 text-indigo-300 border-indigo-500/20',
  'bg-orange-500/20 text-orange-300 border-orange-500/20',
  'bg-pink-500/20 text-pink-300 border-pink-500/20',
  'bg-green-500/10 text-green-400 border-green-500/20',
  'bg-yellow-500/10 text-yellow-400 border-yellow-500/20',
  'bg-purple-500/20 text-purple-300 border-purple-500/20',
  'bg-red-500/20 text-red-300 border-red-500/20',
  'bg-cyan-500/20 text-cyan-300 border-cyan-500/20',
  'bg-rose-500/20 text-rose-300 border-rose-500/20',
  'bg-emerald-500/20 text-emerald-300 border-emerald-500/20',
];

function getDynamicColor(text: string): string {
  let hash = 0;
  for (let i = 0; i < text.length; i++) {
    hash = text.charCodeAt(i) + ((hash << 5) - hash);
  }
  return COLOR_PALETTES[Math.abs(hash) % COLOR_PALETTES.length] as string;
}

/** 
 * Dynamic Tagging Heuristic:
 * Automatically generates a beautiful, stable colored tag for any brand/name.
 */
export function extractDynamicTags(name: string) {
  return [
    { 
      label: name, 
      color: getDynamicColor(name) 
    }
  ];
}

/**
 * Metadata Inference:
 * Sniffs model names/IDs for technical details (params, quant, type).
 * Essential for cloud models where header metadata isn't available.
 */
export function inferMetadata(text: string) {
  const meta: any = {};
  const normalized = text.toLowerCase();
  
  // 1. Parameter Sniffing (e.g., 7b, 32B, 1.5b)
  const paramMatch = text.match(/(\d+(?:\.\d+)?)[bB]\b/);
  if (paramMatch && paramMatch[1]) {
    meta.parameters = parseFloat(paramMatch[1]) * 1_000_000_000;
  }
  
  // 2. Quantization Sniffing (e.g., Q4_K_M, FP16, INT8)
  const quantMatch = text.match(/(Q\d_[Kk]_[A-Za-z]|\b[Ff][Pp]\d+\b|\b[Ii][Nn][Tt]\d+\b)/i);
  if (quantMatch && quantMatch[1]) {
    meta.quantization = quantMatch[1].toUpperCase();
  }

  // 3. Architecture/Role Sniffing
  if (normalized.includes('instruct')) meta.architecture = 'Instruct';
  else if (normalized.includes('coder')) meta.architecture = 'Coder';
  else if (normalized.includes('chat')) meta.architecture = 'Chat';
  else if (normalized.includes('vision')) meta.architecture = 'Vision';
  else if (normalized.includes('math')) meta.architecture = 'Math';
  
  return meta;
}
