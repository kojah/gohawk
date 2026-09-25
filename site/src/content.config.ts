import { defineCollection } from 'astro:content';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob } from 'astro/loaders';
import { z } from 'astro/zod';

const blog = defineCollection({
	loader: glob({ pattern: '**/*.{md,mdx}', base: './src/content/blog' }),
	schema: z.object({
		title: z.string(),
		description: z.string(),
		date: z.date(),
		draft: z.boolean().default(false),
	}),
});

export const collections = {
	blog,
	docs: defineCollection({
		// docs/development holds maintained contributor references that are read
		// in the repository, not published on the site.
		loader: glob({ pattern: ['**/*.{md,mdx}', '!development/**'], base: '../docs' }),
		schema: docsSchema({
			extend: z.object({
				seoTitle: z.string().optional(),
			}),
		}),
	}),
};
