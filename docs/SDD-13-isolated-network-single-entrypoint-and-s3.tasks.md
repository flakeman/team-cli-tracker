# SDD-13 Tasks

- [x] T13-001 Draft SDD-13 scope and acceptance contract.
- [x] T13-002 Add WireGuard deployment contract (`deploy/wireguard.env.example`, installer).
- [x] T13-003 Add gateway deployment on all nodes with bind support.
- [x] T13-004 Add keepalived/VRRP deployment template and installer.
- [x] T13-005 Add distributed MinIO deployment scripts (`install`, `systemd`, `bootstrap`, `healthcheck`).
- [x] T13-006 Wire tracker systemd runtime to cluster env + S3 flags.
- [x] T13-007 Extend cluster env templates for S3 defaults and peer URLs.
- [x] T13-008 Update isolated deployment runbook for WG + gateway + MinIO + VIP.
- [x] T13-009 Update release checklist with failover and S3 gates.
- [x] T13-010 Update changelog with SDD-13 baseline deliverables.
- [x] T13-011 Execute 3-node end-to-end isolated smoke with keepalived VIP and MinIO.
- [x] T13-012 Attach smoke evidence and mark release GO/NO-GO based on gates.
