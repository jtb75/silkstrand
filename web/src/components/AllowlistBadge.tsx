type Status = 'allowlisted' | 'out-of-policy' | 'unknown';

interface Props {
  status: Status;
}

export default function AllowlistBadge({ status }: Props) {
  const label = status === 'allowlisted' ? 'allowlisted' : status === 'out-of-policy' ? 'out of policy' : 'unknown';
  return <span className={`badge badge-allowlist-${status}`}>{label}</span>;
}
