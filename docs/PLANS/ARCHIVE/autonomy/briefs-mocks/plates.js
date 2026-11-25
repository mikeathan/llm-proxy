
const P={
 world:'<circle cx="60" cy="36" r="26"/><ellipse cx="60" cy="36" rx="12" ry="26"/><path d="M34 36h52M38 24h44M38 48h44"/>',
 economy:'<path d="M14 58V40M30 58V30M46 58V44M62 58V22M78 58V34M94 58V16"/><path d="M12 46l20-14 16 10 18-22 22 6" class="a"/>',
 tech:'<rect x="40" y="22" width="40" height="28"/><path d="M50 22v-8M60 22v-8M70 22v-8M50 50v8M60 50v8M70 50v8M40 32h-8M40 42h-8M80 32h8M80 42h8"/><rect x="52" y="30" width="16" height="12" class="a"/>',
 politics:'<path d="M20 58h80M26 58V30M42 58V30M58 58V30M74 58V30M90 58V30M18 30h84L60 12z"/>',
 science:'<ellipse cx="60" cy="36" rx="38" ry="14"/><ellipse cx="60" cy="36" rx="38" ry="14" transform="rotate(60 60 36)"/><ellipse cx="60" cy="36" rx="38" ry="14" transform="rotate(120 60 36)"/><circle cx="60" cy="36" r="4" class="a"/>',
 day:'<circle cx="60" cy="36" r="24"/><path d="M60 20v16l11 7" class="a"/>',
 home:'<path d="M26 58V34l34-20 34 20v24z"/><rect x="52" y="40" width="16" height="18" class="a"/><path d="M96 20c8 6 8 14 0 20M104 14c13 10 13 26 0 36"/>',
 watch:'<path d="M20 52c10-24 20-24 30-8s20 12 50-20"/><circle cx="50" cy="44" r="4" class="a"/>',
};
function plate(k,w=120,h=72){return '<svg class="plate" viewBox="0 0 120 72" width="'+w+'" height="'+h+'" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">'+P[k]+'</svg>'}
document.querySelectorAll('[data-plate]').forEach(e=>e.innerHTML=plate(e.dataset.plate,+e.dataset.w||120,+e.dataset.h||72));
const q=new URLSearchParams(location.search);document.documentElement.dataset.theme=q.get('theme')||'dark';
