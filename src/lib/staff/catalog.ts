import { parseMoney } from '$lib/cart';

export type InventoryCatalogRow = {
	id: string;
	shopName: string;
	artistName: string;
	productCode: string;
	title: string;
	itemType: string;
	productDisplay: string;
	quantity: number | null;
	priceCents: number;
	notes: string;
	soldOut: boolean;
};

export function inventoryItemId(productCode: string, title: string) {
	return `${productCode}::${title}`;
}

export function catalogFromCsvRows(rows: { [key: string]: string }[]): InventoryCatalogRow[] {
	return rows.map((row) => {
		const productCode = (row['Product Code'] ?? '').trim();
		const title = (row['Item Name'] ?? '').trim();
		const qtyRaw = (row['Quantity'] ?? '').trim();
		const qty = qtyRaw === '' ? null : Number.parseInt(qtyRaw.replace(/[^0-9-]/g, ''), 10);
		const price = parseMoney(row['Price per unit (C$ CAD)'] ?? '');

		return {
			id: inventoryItemId(productCode, title),
			shopName: (row['Shop Name'] ?? '').trim(),
			artistName: (row['Artist Name'] ?? '').trim(),
			productCode,
			title,
			itemType: (row['Item Type'] ?? '').trim(),
			productDisplay: (row['Product Display'] ?? '').trim(),
			quantity: Number.isFinite(qty as number) ? (qty as number) : null,
			priceCents: Math.round(price * 100),
			notes: (row['Notes'] ?? '').trim(),
			soldOut: /sold out/i.test(title)
		};
	});
}
