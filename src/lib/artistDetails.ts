export type ArtistDetail = {
  label: string;
  href?: string;
};

export function normalizeArtistKey(value: string) {
  return String(value || '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '');
}

export function parsePromoLinks(raw: string): ArtistDetail[] {
  const text = String(raw || '').trim();
  if (!text || /^n\/?a\.?$/i.test(text)) return [];

  const parts = text
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter(Boolean);

  const details: ArtistDetail[] = [];
  const seen = new Set<string>();

  for (const part of parts) {
    if (/^n\/?a\.?$/i.test(part)) continue;

    let href = part;
    if (!/^https?:\/\//i.test(href) && !href.startsWith('mailto:')) {
      href = `https://${href}`;
    }

    let label = part.replace(/^https?:\/\//i, '').replace(/\/$/, '');
    try {
      const url = new URL(href);
      label = `${url.host}${url.pathname === '/' ? '' : url.pathname}`.replace(/\/$/, '');
    } catch {
      // keep label as cleaned up part
    }

    const id = href.toLowerCase();
    if (seen.has(id)) continue;
    seen.add(id);
    details.push({ label, href });
  }

  return details;
}

function addPromoKeys(
  target: Record<string, ArtistDetail[]>,
  key: string,
  details: ArtistDetail[]
) {
  const normalized = normalizeArtistKey(key);
  if (!normalized || !details.length) return;
  if (!target[normalized]) target[normalized] = details;
}

export function buildArtistPromoIndex(
  rows: { [key: string]: string }[]
): Record<string, ArtistDetail[]> {
  const index: Record<string, ArtistDetail[]> = {};

  for (const row of rows) {
    const details = parsePromoLinks(row['Artist Promo Links'] || '');
    if (!details.length) continue;

    addPromoKeys(index, row['Artist Name'] || '', details);
    addPromoKeys(index, row['Your Name'] || '', details);

    const artistName = String(row['Artist Name'] || '');
    const parenMatch = artistName.match(/\(([^)]+)\)/);
    if (parenMatch) addPromoKeys(index, parenMatch[1], details);
    for (const part of artistName.split(/\s*[&|/]\s*/)) {
      addPromoKeys(index, part, details);
    }

    const discord = String(row['Discord Handle'] || '');
    for (const handle of discord.split(/\s+and\s+|,|\//i)) {
      addPromoKeys(index, handle.replace(/^@/, ''), details);
    }

    for (const detail of details) {
      try {
        const host = new URL(detail.href || '').hostname.replace(/^www\./, '');
        const hostRoot = host.split('.')[0] || '';
        addPromoKeys(index, hostRoot, details);
        addPromoKeys(index, host, details);
      } catch {
        // ignore invalid urls
      }
    }
  }

  return index;
}

export function artistDetailsFromPromos(
  name: string,
  promoIndex: Record<string, ArtistDetail[]>
): ArtistDetail[] {
  const candidates = [
    name,
    ...String(name || '')
      .split(/\s*[&|/]\s*/)
      .map((part) => part.trim())
      .filter(Boolean)
  ];

  for (const candidate of candidates) {
    const key = normalizeArtistKey(candidate);
    if (!key) continue;
    if (promoIndex[key]) return promoIndex[key];

    for (const [alias, details] of Object.entries(promoIndex)) {
      if (alias.length < 4 || key.length < 4) continue;

      // longer shop or name starts with a shorter artist alias
      if (key.startsWith(alias)) return details;

      if (key.length >= 6 && alias.includes(key)) return details;
    }
  }

  return [];
}

export function shopPromoDetails(
  shopName: string,
  _rows: { [key: string]: string }[] = [],
  promoIndex: Record<string, ArtistDetail[]>
): ArtistDetail[] {
  // use the shops own promo match. don't get links from other artists that share the same product code prefix
  return artistDetailsFromPromos(shopName, promoIndex);
}
