import { redirect } from '@sveltejs/kit';

export async function load() {
	throw redirect(
		302,
		'https://raw.githubusercontent.com/klirix/jig/master/packages/server/init.sh'
	);
}
