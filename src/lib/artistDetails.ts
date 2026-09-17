export function artistSlug(name: string) {
	return name.toLowerCase().replace(/[^a-z0-9]+/g, '').slice(0, 18) || 'artist';
}

export function artistDetails(name: string) {
	const handle = artistSlug(name);
	let hash = 0;

	for (let i = 0; i < name.length; i++) {
		hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
	}

	switch (hash % 8) {
		case 0:
			return [];
		case 1:
			return [{ label: `Discord: @${handle}` }];
		case 2:
			return [{ label: `${handle}@example.com`, href: `mailto:${handle}@example.com` }];
		case 3:
			return [{ label: `https://${handle}.example.com`, href: `https://${handle}.example.com` }];
		case 4:
			return [
				{ label: `Discord: @${handle}` },
				{ label: `${handle}@example.com`, href: `mailto:${handle}@example.com` }
			];
		case 5:
			return [
				{ label: `Discord: @${handle}` },
				{ label: `https://instagram.com/${handle}`, href: `https://instagram.com/${handle}` }
			];
		case 6:
			return [
				{ label: `${handle}@example.com`, href: `mailto:${handle}@example.com` },
				{ label: `https://${handle}.shop`, href: `https://${handle}.shop` }
			];
		default:
			return [
				{ label: `Discord: @${handle}` },
				{ label: `${handle}@example.com`, href: `mailto:${handle}@example.com` },
				{ label: `https://${handle}.art`, href: `https://${handle}.art` },
				{ label: `https://instagram.com/${handle}`, href: `https://instagram.com/${handle}` }
			];
	}
}
