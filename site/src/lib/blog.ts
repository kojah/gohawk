import { getCollection } from 'astro:content';

export async function getPublishedPosts() {
	const posts = await getCollection('blog', ({ data }) => !data.draft);
	return posts.sort((left, right) => right.data.date.getTime() - left.data.date.getTime());
}

export function formatBlogDate(date: Date): string {
	return new Intl.DateTimeFormat('en', { dateStyle: 'long' }).format(date);
}
