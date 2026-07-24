// potato update (spec §8): CLI-only, latest-only. Verifies sha256 against the
// released SHA256SUMS with install.sh's rigor, atomically renames over the
// running binary's realpath, regenerates init files. Never touches the rc.

import { createHash } from 'node:crypto';
import { chmodSync, copyFileSync, mkdtempSync, realpathSync, renameSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { binPath, potatoDir } from './paths';
import { writeInitFiles } from './shell';
import { VERSION } from './version';

export const REPO = 'luojiahai/potato';

export function parseSha256Sums(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split('\n')) {
    const m = line.trim().match(/^([0-9a-fA-F]+)\s+\*?(.+)$/);
    if (m) out[m[2]!] = m[1]!;
  }
  return out;
}

export function targetTriple(platform: string, arch: string): string {
  if ((platform === 'darwin' || platform === 'linux') && (arch === 'x64' || arch === 'arm64'))
    return `${platform}-${arch}`;
  throw new Error(`unsupported platform: ${platform}-${arch}`);
}

async function latestTag(): Promise<string> {
  const res = await fetch(`https://github.com/${REPO}/releases/latest`, { redirect: 'manual' });
  const location = res.headers.get('location');
  const tag = location?.match(/\/tag\/([^/]+)$/)?.[1];
  if (!tag) throw new Error(`could not resolve the latest release from github.com/${REPO}`);
  return tag;
}

async function download(url: string): Promise<Buffer> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`download failed (${res.status}): ${url}`);
  return Buffer.from(await res.arrayBuffer());
}

export async function runUpdate(): Promise<void> {
  if (VERSION === 'dev') throw new Error('update only works on an installed release build');
  const tag = await latestTag();
  const latest = tag.replace(/^v/, '');
  if (latest === VERSION) {
    console.log(`potato ${VERSION} is already up to date.`);
    return;
  }

  const triple = targetTriple(process.platform, process.arch);
  const asset = `potato-${triple}.tar.gz`;
  const base = `https://github.com/${REPO}/releases/download/${tag}`;
  console.log(`updating potato ${VERSION} → ${latest}…`);

  const [archive, sumsText] = await Promise.all([
    download(`${base}/${asset}`),
    download(`${base}/SHA256SUMS`).then((b) => b.toString('utf8')),
  ]);

  const expected = parseSha256Sums(sumsText)[asset];
  if (!expected) throw new Error(`SHA256SUMS has no entry for ${asset}`);
  const actual = createHash('sha256').update(archive).digest('hex');
  if (actual !== expected.toLowerCase())
    throw new Error(`sha256 mismatch for ${asset}: expected ${expected}, got ${actual}`);

  const work = mkdtempSync(join(tmpdir(), 'potato-update-'));
  try {
    const tarball = join(work, asset);
    await Bun.write(tarball, archive);
    const tar = spawnSync('tar', ['-xzf', tarball, '-C', work]);
    if (tar.status !== 0) throw new Error(`tar failed: ${tar.stderr?.toString() ?? 'unknown error'}`);

    const extracted = join(work, 'potato');
    chmodSync(extracted, 0o755);
    // atomic swap over the RUNNING binary's realpath (spec §8) — execPath,
    // not the env-derived install dir, which may differ in this shell;
    // stage next to the target first so the rename never crosses filesystems
    const target = realpathSync(process.execPath);
    const staged = join(dirname(target), '.potato.new');
    copyFileSync(extracted, staged);
    chmodSync(staged, 0o755);
    renameSync(staged, target);
    writeInitFiles(binPath(), potatoDir());
    console.log(`potato ${latest} installed at ${target}`);
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}
