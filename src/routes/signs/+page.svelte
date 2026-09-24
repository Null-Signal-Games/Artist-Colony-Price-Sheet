<script lang="ts">
  import { shopPromoDetails } from '$lib/artistDetails';
  import { shopQrUrl } from '$lib/shopQrCodes';

  export let data: {
    csvData: { [key: string]: string }[];
    shopNames?: Set<string>;
    artistNames: Set<string>;
    artistPromos?: Record<string, { label: string; href?: string }[]>;
  };

  const shopColumnKey = 'Shop Name';
  const artistColumnKey = 'Artist Name';

  const shopOrder = new Map<string, number>();
  data.csvData.forEach((row, index) => {
    const shop = (row[shopColumnKey] || row[artistColumnKey] || '').trim();
    if (shop && !shopOrder.has(shop)) shopOrder.set(shop, index);
  });

  const shops = Array.from(data.shopNames ?? data.artistNames)
    .filter(Boolean)
    .sort((a, b) => {
      const orderA = shopOrder.get(a) ?? Number.MAX_SAFE_INTEGER;
      const orderB = shopOrder.get(b) ?? Number.MAX_SAFE_INTEGER;
      if (orderA !== orderB) return orderA - orderB;
      return a.localeCompare(b);
    })

  const SHOPS_PER_SHEET = 4;
  const promoIndex = data.artistPromos ?? {};

  // artist name if it differs from shop name
  const SHOP_PROMO_ALIASES: Record<string, string> = {
    'MTL Netrunner': 'Montreal Netrunner',
    'ManintheMoon & Friends': 'Netrunner cards.Net (ManintheMoon)',
    Elessar_Ellie: 'Mr. Ellie',
    HelaBellaArt: 'ksodiya',
  }

  type SignTile = {
    label: string;
    shop: string;
    urls: string[];
    qrUrl?: string;
    flipped: boolean;
  };

  function shopUrls(shop: string) {
    if (!shop) return [];
    const names = [shop, SHOP_PROMO_ALIASES[shop]].filter(Boolean) as string[];
    for (const name of names) {
      const details = shopPromoDetails(name, [], promoIndex)
        .map((detail) => detail.label)
        .filter(Boolean);
      if (details.length) return details;
    }
    return [];
  }

  function shopCard(shop: string): Pick<SignTile, 'shop' | 'urls' | 'qrUrl'> {
    const urls = shopUrls(shop);
    return {
      shop,
      urls,
      qrUrl: urls.length ? shopQrUrl(shop) : undefined
    };
  }

  function sheetTiles(sheetShops: string[]): SignTile[] {
    const [a = '', b = '', c = '', d = ''] = sheetShops;
    const cardA = shopCard(a);
    const cardB = shopCard(b);
    const cardC = shopCard(c);
    const cardD = shopCard(d);
    // A–H left-to-right, top-to-bottom. Pairs: A/C, B/D, E/G, F/H
    // top of each pair is upside-down so folded tents read correctly
    return [
      { label: 'A', ...cardA, flipped: true },
      { label: 'B', ...cardB, flipped: true },
      { label: 'C', ...cardA, flipped: false },
      { label: 'D', ...cardB, flipped: false },
      { label: 'E', ...cardC, flipped: true },
      { label: 'F', ...cardD, flipped: true },
      { label: 'G', ...cardC, flipped: false },
      { label: 'H', ...cardD, flipped: false }
    ];
  }

  const sheets: SignTile[][] = [];
  for (let i = 0; i < shops.length; i += SHOPS_PER_SHEET) {
    sheets.push(sheetTiles(shops.slice(i, i + SHOPS_PER_SHEET)));
  }

  function printSigns() {
    window.print();
  }
</script>

<svelte:head>
  <title>Artist Colony 2026 Signs</title>
</svelte:head>

<div class="print-preview-page">
  <header class="print-toolbar">
    <a class="print-back-link" href="/">← Price sheet</a>
    <p class="print-toolbar-hint">
      Letter · 2×4 tent cards · {shops.length} shops · {sheets.length} sheets
    </p>
    <button type="button" class="print-pdf-button" on:click={printSigns}>Print / Save PDF</button>
  </header>

  <div class="print-sheet-wrap">
    {#each sheets as tiles, sheetIndex}
      {#if sheetIndex > 0}
        <div class="print-break-marker" aria-hidden="true">Page break</div>
      {/if}
      <section class="signs-page">
        <div class="signs-grid" aria-label="Shop name card grid">
          {#each tiles as tile}
            <div class="signs-tile" data-tile={tile.label}>
              <div
                class="signs-tile-face"
                class:signs-tile-flipped={tile.flipped}
                class:signs-tile-name-only={tile.shop && !tile.urls.length}
              >
                {#if tile.shop}
                  <p class="signs-shop-name">{tile.shop}</p>
                  {#if tile.urls.length}
                    {#if tile.qrUrl}
                      <img class="signs-shop-qr" src={tile.qrUrl} alt="" width="88" height="88" />
                    {/if}
                    <ul class="signs-shop-urls">
                      {#each tile.urls as url}
                        <li>{url}</li>
                      {/each}
                    </ul>
                  {/if}
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </section>
    {/each}
  </div>
</div>
