// PROTOTYPE — seed data, in memory only.

export interface Command {
  name: string;
  command: string;
  description?: string;
}

export interface CommandState {
  lastUsedAt: number;
  args?: Record<string, string>;
}

const now = Date.now();
const min = 60_000;

export const seedCommands: Command[] = [
  {
    name: 'k8s-shell',
    command: 'kubectl exec -it {{pod}} -n {{namespace=default}} -- /bin/bash',
    description: 'Open a shell inside a pod',
  },
  {
    name: 'docker-nuke',
    command: 'docker system prune -af --volumes',
    description: 'Remove all unused containers, images, volumes',
  },
  {
    name: 'pg-dump-prod',
    command:
      'pg_dump "postgres://readonly@prod-db.internal:5432/{{database=app}}" --no-owner -Fc -f {{database=app}}-$(date +%F).dump',
    description: 'Snapshot a prod database to a local dump file',
  },
  {
    name: 'ssh-tunnel-grafana',
    command: 'ssh -N -L {{local_port=3000}}:localhost:3000 ops@{{host}}',
    description: 'Tunnel Grafana from a remote box to localhost',
  },
  {
    name: 'git-fixup-last',
    command: 'git commit --amend --no-edit && git push --force-with-lease',
    description: 'Fold staged changes into the last commit and push',
  },
  {
    name: 'aws-logs-tail',
    command:
      'aws logs tail /ecs/{{service}} --follow --since 15m --region {{region=ap-southeast-2}}',
    description: 'Tail ECS service logs',
  },
  {
    name: 'ffmpeg-gif',
    command:
      'ffmpeg -i {{input}} -vf "fps=12,scale=720:-1:flags=lanczos" -loop 0 {{input}}.gif',
    description: 'Convert a video to a looping gif',
  },
  {
    name: 'listening-ports',
    command: 'lsof -iTCP -sTCP:LISTEN -n -P',
    description: 'Show every process listening on a TCP port',
  },
];

// Simulates ~/.potato/state.json: MRU order + last-used arg values.
export const seedState: Record<string, CommandState> = {
  'k8s-shell': {
    lastUsedAt: now - 5 * min,
    args: { pod: 'api-7d9f8b6c4-x2j5v', namespace: 'staging' },
  },
  'aws-logs-tail': { lastUsedAt: now - 30 * min, args: { service: 'api' } },
  'git-fixup-last': { lastUsedAt: now - 2 * 60 * min },
  'listening-ports': { lastUsedAt: now - 24 * 60 * min },
};
