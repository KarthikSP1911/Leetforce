<p align="center">
  <img src="web/public/brand/logo-mark.svg" alt="LeetForce logo" width="120">
</p>

<h1 align="center">LeetForce</h1>

<p align="center">
  A distributed, sandboxed code execution platform. Submit Python, C++, Java, or Go and get a verdict with runtime and memory.
</p>

## Cost table

Recurring costs that are billed while the resource exists or runs. Prices depend on the AWS region; check the AWS pricing pages before relying on a number.

| Resource | Introduced | Billing | Notes |
|---|---|---|---|
| EC2 `t3.micro` dev host (`leetforce-dev`) | Phase 1 | Per hour while running | Stop it when idle. t3 defaults to unlimited CPU-credit mode, so sustained CPU can add surplus-credit charges. See [ADR 0003](docs/adr/0003-dev-environment-ec2-x86.md). |
| EBS gp3 volume, 15 GiB | Phase 1 | Per GB-month, also while the instance is stopped | Deleted only when the instance is terminated. |
| Public IPv4 address | Phase 1 | Per hour while attached (AWS charges for public IPv4) | The address changes on stop/start unless an Elastic IP is attached; an unattached Elastic IP is also billed. |
