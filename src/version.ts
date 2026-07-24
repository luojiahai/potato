// Release builds bake the version in via
// `bun build --define process.env.POTATO_VERSION='"x.y.z"'` (scripts/build.sh).
export const VERSION: string = process.env.POTATO_VERSION ?? 'dev';
