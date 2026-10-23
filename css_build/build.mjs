import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { brotliCompressSync, constants, gzipSync } from 'node:zlib';
import CleanCSS from 'clean-css';

const root = new URL('./', import.meta.url);
const output = new URL('build/', root);
const sources = [
  'node_modules/normalize.css/normalize.css',
  'node_modules/github-markdown-css/github-markdown.css',
  'src/blog.css',
];
const styles = await Promise.all(sources.map(path => readFile(new URL(path, root), 'utf8')));
// Font paths are relative to the deployed stylesheet in assets/, not the sources.
const result = new CleanCSS({ rebase: false }).minify(styles.join('\n'));
if (result.errors.length) {
  throw new Error(result.errors.join('\n'));
}
for (const warning of result.warnings) console.warn(warning);

const assets = new Map([
  ['blog-9.css', Buffer.from(result.styles)],
  ['favicon.ico', await readFile(new URL('src/favicon.ico', root))],
]);
// This fixed directory contains only generated assets.
await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
for (const [name, content] of assets) {
  await writeFile(new URL(name, output), content);
  await writeFile(new URL(`${name}.gz`, output), gzipSync(content, { level: 9 }));
  await writeFile(new URL(`${name}.br`, output), brotliCompressSync(content, {
    params: { [constants.BROTLI_PARAM_QUALITY]: 11 },
  }));
}
console.log(`Built ${assets.size} assets with gzip and Brotli variants in ${fileURLToPath(output)}`);
