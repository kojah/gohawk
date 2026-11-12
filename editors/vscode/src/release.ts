// Naming and verification of gohawk release archives. Pure functions, so the
// download path can be tested without a network or an editor.

/** The gohawk release this extension version was tested with. */
export const pinnedVersion = '0.8.0';

export type ReleaseTarget = {
	/** Archive name without extension; also the directory inside the archive. */
	name: string;
	archive: string;
	executable: string;
};

const systems: Readonly<Record<string, string>> = {
	linux: 'linux',
	darwin: 'darwin',
	win32: 'windows',
};
const architectures: Readonly<Record<string, string>> = { x64: 'amd64', arm64: 'arm64' };

export function executableName(platform: string): string {
	return platform === 'win32' ? 'gohawk.exe' : 'gohawk';
}

/** The archive published for a Node platform and architecture, if one is. */
export function releaseTarget(
	version: string,
	platform: string,
	architecture: string,
): ReleaseTarget | undefined {
	const system = systems[platform];
	const machine = architectures[architecture];
	if (system === undefined || machine === undefined) {
		return undefined;
	}
	const name = `gohawk_${version}_${system}_${machine}`;
	return {
		name,
		archive: name + (system === 'windows' ? '.zip' : '.tar.gz'),
		executable: executableName(platform),
	};
}

/** The SHA-256 a checksums.txt file records for one archive. */
export function expectedChecksum(checksums: string, archive: string): string | undefined {
	for (const line of checksums.split('\n')) {
		const [digest, name] = line.trim().split(/\s+/);
		if (name === archive && digest !== undefined && /^[0-9a-f]{64}$/.test(digest)) {
			return digest;
		}
	}
	return undefined;
}

export function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}
