
/** Regex to identify where a "Base Name" ends and "Version/Quant/Size" starts */
export const MODEL_CLEANUP_REGEX = /[-\._]([qQ][0-9]|[0-9]+[bB]|[vV][0-9]+)[a-zA-Z0-9_]*/gi;

/** Normalizes a filename to its base family name */
export function getBaseName(filename: string): string {
  return filename
    .replace(/\.gguf$/i, '')
    .replace(MODEL_CLEANUP_REGEX, '')
    .trim();
}

/** Extracts Size and Quantization into a compact label */
export function getVariantLabel(filename: string): string {
  const sizeMatch = filename.match(/[0-9]+[bB]/);
  const quantMatch = filename.match(/[qQ][0-9][a-zA-Z0-9_]*/);
  
  const size = sizeMatch ? sizeMatch[0].toUpperCase() : '';
  const quant = quantMatch ? quantMatch[0].toUpperCase() : '';
  
  return size && quant ? `${size} • ${quant}` : size || quant || 'Original';
}

/** 
 * Dynamic Tagging Heuristic:
 * Extracts brand names and version numbers effectively.
 */
export function extractDynamicTags(name: string) {
  const keywords = [
    { label: 'Llama', color: 'bg-blue-500/20 text-blue-300 border-blue-500/20', match: /llama/i },
    { label: 'Gemma', color: 'bg-teal-500/20 text-teal-300 border-teal-500/20', match: /gemma/i },
    { label: 'Qwen', color: 'bg-indigo-500/20 text-indigo-300 border-indigo-500/20', match: /qwen/i },
    { label: 'Mistral', color: 'bg-orange-500/20 text-orange-300 border-orange-500/20', match: /mistral/i },
    { label: 'Phi', color: 'bg-pink-500/20 text-pink-300 border-pink-500/20', match: /phi/i },
    { label: 'Instruct', color: 'bg-green-500/10 text-green-400 border-green-500/20', match: /instruct/i },
    { label: 'Coder', color: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/20', match: /coder/i },
  ];

  const tags = keywords
    .filter(k => k.match.test(name))
    .map(k => ({ label: k.label, color: k.color }));

  // Also auto-extract any "3.1", "3.5" style versions
  const versionMatch = name.match(/[0-9]+\.[0-9]+/);
  if (versionMatch) {
    tags.push({ 
        label: `v${versionMatch[0]}`, 
        color: 'bg-gray-500/20 text-gray-400 border-gray-500/10' 
    });
  }

  return tags;
}
