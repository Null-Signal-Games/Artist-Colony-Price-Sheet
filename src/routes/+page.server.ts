import { parse } from 'csv-parse/sync';
import dayjs from 'dayjs';
import localizedFormat from 'dayjs/plugin/localizedFormat';
import utc from 'dayjs/plugin/utc';
import timezone from 'dayjs/plugin/timezone';
import { readFileSync, statSync } from 'fs';
import path from 'path';
import { buildArtistPromoIndex, type ArtistDetail } from '$lib/artistDetails';

dayjs.extend(localizedFormat);
dayjs.extend(utc);
dayjs.extend(timezone);

type CSVRow = { [key: string]: string };

const INVENTORY_FILE = 'w26-inventory.csv';
const ARTISTS_FILE = 'w26-artists.csv';

const NORMALIZED_COLUMNS = [
	'Shop Name',
	'Artist Name',
	'Product Code',
	'Item Name',
	'Item Type',
	'Product Display',
	'Quantity',
	'Price per unit (C$ CAD)',
	'Notes'
] as const;

function findColumnKey(keys: string[], match: (key: string) => boolean) {
	return keys.find((key) => match(key.replace(/\s+/g, ' ').trim()));
}

function normalizeInventoryRows(rows: CSVRow[]): CSVRow[] {
	if (!rows.length) return rows;

	const keys = Object.keys(rows[0]);
	const shopKey = findColumnKey(keys, (key) => key === 'Shop Name');
	const artistKey = findColumnKey(keys, (key) => key.startsWith('Artist Name')) ?? keys[0];
	const productCodeKey = findColumnKey(keys, (key) => key === 'Product Code') ?? 'Product Code';
	const itemNameKey = findColumnKey(keys, (key) => key === 'Item Name') ?? 'Item Name';
	const itemTypeKey = findColumnKey(keys, (key) => key === 'Item Type') ?? 'Item Type';
	const productDisplayKey =
		findColumnKey(keys, (key) => key === 'Product Display') ?? 'Product Display';
	const quantityKey = findColumnKey(keys, (key) => key === 'Quantity') ?? 'Quantity';
	const priceKey =
		findColumnKey(keys, (key) => key.startsWith('Price per unit')) ?? 'Price per unit (C$ CAD)';
	const notesKey = findColumnKey(keys, (key) => key === 'Notes') ?? 'Notes';

	return rows.map((row) => {
		const artistName = String(row[artistKey] ?? '').trim();
		return {
			'Shop Name': String((shopKey ? row[shopKey] : '') ?? '').trim() || artistName,
			'Artist Name': artistName,
			'Product Code': String(row[productCodeKey] ?? '').trim(),
			'Item Name': String(row[itemNameKey] ?? '').trim(),
			'Item Type': String(row[itemTypeKey] ?? '').trim(),
			'Product Display': String(row[productDisplayKey] ?? '').trim(),
			Quantity: String(row[quantityKey] ?? '').trim(),
			'Price per unit (C$ CAD)': String(row[priceKey] ?? '').trim(),
			Notes: String(row[notesKey] ?? '').trim()
		};
	});
}

function loadCsv(fileName: string): CSVRow[] {
	const csvFilePath = path.resolve(process.cwd(), 'src', 'data', fileName);
	const fileContent = readFileSync(csvFilePath, 'utf-8');
	return parse(fileContent, {
		columns: true,
		skip_empty_lines: true,
		relax_column_count: true,
		bom: true,
		relax_quotes: true
	});
}

export const prerender = true;
export async function load() {
	const inventoryPath = path.resolve(process.cwd(), 'src', 'data', INVENTORY_FILE);
	const parsedInventory = loadCsv(INVENTORY_FILE);
	const parsedArtists = loadCsv(ARTISTS_FILE);

	// scrub the artists CSV down to only the necessary columns
	// so pii isn't included in the distributable build
	const promoSourceRows = parsedArtists.map((row) => ({
		'Artist Promo Links': String(row['Artist Promo Links'] ?? ''),
		'Artist Name': String(row['Artist Name'] ?? ''),
		'Your Name': String(row['Your Name'] ?? ''),
		'Discord Handle': String(row['Discord Handle'] ?? '')
	}));

	const csvData = normalizeInventoryRows(parsedInventory).filter((row) =>
		NORMALIZED_COLUMNS.some((column) => row[column])
	);

	const artistPromos: Record<string, ArtistDetail[]> = buildArtistPromoIndex(promoSourceRows);

	const shopNames = new Set(csvData.map((row) => row['Shop Name']).filter((name) => Boolean(name)));
	const lastUpdated = dayjs(statSync(inventoryPath).mtimeMs)
		.tz('America/New_York')
		.format('dddd YYYY-MM-DD [at] h:mma');

	return {
		csvData,
		lastUpdated,
		shopNames,
		artistNames: shopNames,
		artistPromos
	};
}
