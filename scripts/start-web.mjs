import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('../', import.meta.url));
const config = path.join(root, 'dist/server/wrangler.json');
const vars = path.join(root, '.dev.vars');
if (!existsSync(config) || !existsSync(vars)) {
  console.error('Run npm run build and configure .dev.vars before npm start.');
  process.exit(1);
}
// Wrangler resolves worker env files relative to the generated config directory.
// Use an absolute path so secrets remain outside dist and deployment archives.
const child = spawn(
  process.execPath,
  [
    path.join(root, 'node_modules/wrangler/bin/wrangler.js'),
    'dev',
    '--config',
    config,
    '--env-file',
    vars,
    '--ip',
    '127.0.0.1',
    '--port',
    '5179',
  ],
  { cwd: root, stdio: 'inherit' },
);
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.once(signal, () => child.kill(signal));
}
child.once('error', (error) => {
  console.error(error.message);
  process.exit(1);
});
child.once('exit', (code) => process.exit(code ?? 1));
