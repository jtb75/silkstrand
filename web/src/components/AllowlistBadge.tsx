import type { AllowlistStatus } from '../api/types';

interface Props {
  status: AllowlistStatus;
}

const LABEL: Record<AllowlistStatus, string> = {
  allowlisted: 'allowlisted',
  out_of_policy: 'out of policy',
  unknown: 'unknown',
};

const CSS_SUFFIX: Record<AllowlistStatus, string> = {
  allowlisted: 'allowlisted',
  out_of_policy: 'out-of-policy',
  unknown: 'unknown',
};

export default function AllowlistBadge({ status }: Props) {
  return <span className={`badge badge-allowlist-${CSS_SUFFIX[status]}`}>{LABEL[status]}</span>;
}
