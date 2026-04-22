/** Regex to identify where a "Base Name" ends and "Version/Quant/Size" starts */
export const MODEL_CLEANUP_REGEX = /[-\._]([qQ][0-9]|i[qQ][0-9]|[0-9]+[bB]|[vV][0-9]+|[iI][tT]|draft|preview|instruct|instrcut|f[0-9]|fp[0-9]|int[0-9]|[0-9])[a-zA-Z0-9_\.]*/gi;

/** Normalizes a filename to its base family name for grouping */
export function getBaseName(filename: string): string {
  // Purely algorithmic: Extract the first continuous sequence of letters.
  // This automatically handles 'Qwen2.5' -> 'Qwen', 'deepseek-coder' -> 'Deepseek'
  // without relying on ANY hardcoded brand strings or dictionaries.
  const match = filename.match(/^[a-zA-Z]+/);
  
  let base = match ? match[0] : filename.split(/[-\._]/)[0];
  if (!base) base = 'Unknown';
  
  return base.charAt(0).toUpperCase() + base.slice(1).toLowerCase();
}

/** Formats bytes into a human-readable size (e.g. 4.2 GB) */
export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

/** Extracts Size and Quantization into a compact label */
export function getVariantLabel(filename: string): string {
  const sizeMatch = filename.match(/[0-9]+[bB]/);
  const quantMatch = filename.match(/[qQ][0-9][a-zA-Z0-9_]*/);
  
  const size = sizeMatch ? sizeMatch[0].toUpperCase() : '';
  const quant = quantMatch ? quantMatch[0].toUpperCase() : '';
  
  return size && quant ? `${size} • ${quant}` : size || quant || 'Original';
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
