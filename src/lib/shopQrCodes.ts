import { normalizeArtistKey } from '$lib/artistDetails';

const qrModules = import.meta.glob('../data/Artist Links QR/*.png', {
  eager: true,
  query: '?url',
  import: 'default'
}) as Record<string, string>;

const SHOP_QR_ALIASES: Record<string, string> = {
  maninthemoonfriends: 'maninthem00n',
  maninthemoon: 'maninthem00n',
  mtlnetrunner: 'montrealnetrunner',
  nullsignalgames: 'nsg',
  elessarellie: 'mrellie',
  mrellie: 'mrellie',
  helabellaart: 'ksodiya',
  ksodiya: 'ksodiya'
};

function qrStemFromPath(path: string) {
  const file = path.split('/').pop() ?? path;
  return file
    .replace(/\.png$/i, '')
    .replace(/^w26-ac-/i, '')
    .replace(/^w26-/i, '');
}

const qrByStem = new Map<string, string>();
for (const [path, url] of Object.entries(qrModules)) {
  const stem = normalizeArtistKey(qrStemFromPath(path));
  if (stem) qrByStem.set(stem, url);
}

function candidatesForShop(shopName: string): string[] {
  const raw = String(shopName || '').trim();
  if (!raw) return [];

  const parts = [
    raw,
    ...raw.split(/\s*[&|/]\s*/),
    ...raw.split(/\s+-\s+/),
    ...(raw.match(/\(([^)]+)\)/)?.[1] ? [raw.match(/\(([^)]+)\)/)![1]] : [])
  ]
    .map((part) => part.trim())
    .filter(Boolean);

  const keys = new Set<string>();
  for (const part of parts) {
    const key = normalizeArtistKey(part);
    if (key) keys.add(key);
    const alias = SHOP_QR_ALIASES[key];
    if (alias) keys.add(normalizeArtistKey(alias));
  }
  return [...keys];
}

export function shopQrUrl(shopName: string): string | undefined {
  for (const key of candidatesForShop(shopName)) {
    const direct = qrByStem.get(key);
    if (direct) return direct;

    for (const [stem, url] of qrByStem) {
      if (stem.length < 4 || key.length < 4) continue;
      if (key.startsWith(stem) || stem.startsWith(key)) return url;
      if (key.length >= 6 && stem.includes(key)) return url;
      if (stem.length >= 6 && key.includes(stem)) return url;
    }
  }
  return undefined;
}
