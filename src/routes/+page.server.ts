import { parse } from 'csv-parse/sync';
import dayjs from 'dayjs';
import localizedFormat from 'dayjs/plugin/localizedFormat';
import utc from 'dayjs/plugin/utc';
import timezone from 'dayjs/plugin/timezone';
import { readFileSync, statSync } from 'fs';
import path from 'path';

dayjs.extend(localizedFormat);
dayjs.extend(utc);
dayjs.extend(timezone);

export async function load() {
	const csvFilePath = path.resolve(
		process.cwd(),
		'src',
		'data',
		'Worlds 2025 Artist Colony Master Inventory - Formatted Inventory - FOR PRINT.csv'
	);
	const fileContent = readFileSync(csvFilePath, 'utf-8');

	const csvData = parse(fileContent, {
		columns: true,
		skip_empty_lines: true
	});

	const lastUpdated = dayjs(statSync(csvFilePath).mtimeMs).tz('Europe/London').format('LLLL');

	return {
		csvData,
		lastUpdated
	};
}
