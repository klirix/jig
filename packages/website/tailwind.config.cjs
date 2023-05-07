import path from 'path';
/** @type {import('tailwindcss').Config} */
export default {
	content: [
		'./src/**/*.{html,js,svelte,ts}',
		// 2. Append the path for the Skeleton NPM package and files:
		path.join(require.resolve('@skeletonlabs/skeleton'), '../**/*.{html,js,svelte,ts}')
	],
	theme: {
		extend: {}
	},
	// eslint-disable-next-line @typescript-eslint/no-var-requires
	plugins: [...require('@skeletonlabs/skeleton/tailwind/skeleton.cjs')()]
};
