import rss from '@astrojs/rss';
import { getPublishedPosts } from '../../lib/blog';

export async function GET(context: { site?: URL }) {
	const posts = await getPublishedPosts();

	return rss({
		title: 'gohawk blog',
		description: 'Updates and technical notes from the gohawk project.',
		site: context.site ?? 'https://gohawk.dev',
		items: posts.map((post) => ({
			title: post.data.title,
			description: post.data.description,
			pubDate: post.data.date,
			link: `/blog/${post.id}/`,
		})),
	});
}
