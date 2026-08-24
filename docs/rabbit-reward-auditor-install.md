# Installing the Auditor on WSL

This package contains only new auditor files. It does not replace or modify
files in `consensus/lqc`.

After downloading `rabbit-reward-auditor-archive-lab-1.2.1.tar.gz` on Windows,
run the following commands in WSL:

```bash
PACKAGE="$(find /mnt/c/Users -type f -name 'rabbit-reward-auditor-archive-lab-1.2.1.tar.gz' -print -quit 2>/dev/null)"
tar -xzf "$PACKAGE" -C ~/projects/rabbit-geth
cd ~/projects/rabbit-geth
./scripts/rabbit-devnet/run-reward-audit.sh
```

The lab may remain running. The script creates `build/bin/rabbit-audit` and
writes reports to `audit-reports/YYYYmmdd-HHMMSS/`.

If the overall status is `FAIL`, check the next two lines in the summary.
`Runtime rewards` indicates whether blocks and the Reward Locker matched;
`Consensus architecture` indicates whether structural blockers remain. The
program does not modify consensus.
