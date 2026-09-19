import { defineUnlighthouseConfig } from 'unlighthouse/config';

export default defineUnlighthouseConfig({
	chrome: {
		// Browser archives currently pass through an unmaintained ZIP extractor.
		// CI provides Chrome, so never download and extract a browser at runtime.
		useDownloadFallback: false,
		useSystem: true,
	},
	ci: {
		// Performance remains visible in the report but is intentionally not a gate:
		// its lab score varies with the load on shared CI runners.
		budget: {
			// These floors sit below the first recorded baseline (92 accessibility,
			// 100 best practices, and 91 SEO) and fail only on a real regression.
			accessibility: 90,
			'best-practices': 95,
			seo: 90,
		},
		reporter: 'jsonExpanded',
	},
	outputPath: '.unlighthouse',
	scanner: {
		// The audit is intentionally exhaustive: every URL emitted by the sitemap
		// is small enough to scan, so route sampling would hide regressions.
		dynamicSampling: false,
	},
});
