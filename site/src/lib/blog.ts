import { getCollection } from 'astro:content';

interface BlogPostOptions {
	includeDrafts?: boolean;
}

export async function getBlogPosts({ includeDrafts = false }: BlogPostOptions = {}) {
	const posts = await getCollection('blog', ({ data }) => includeDrafts || !data.draft);
	return posts.sort((left, right) => right.data.date.getTime() - left.data.date.getTime());
}

export function formatBlogDate(date: Date): string {
	return new Intl.DateTimeFormat('en', { dateStyle: 'long' }).format(date);
}
