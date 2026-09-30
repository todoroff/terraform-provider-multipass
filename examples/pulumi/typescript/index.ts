import * as pulumi from "@pulumi/pulumi";
import * as multipass from "@pulumi/multipass";

const config = new pulumi.Config();
const instanceName = config.get("instanceName") ?? `pulumi-ts-${pulumi.getStack()}`;
const hostPath = config.get("hostPath");
const network = config.get("network");

const provider = new multipass.Provider("local", {
    multipassPath: config.get("multipassPath"),
    commandTimeout: 1200,
    defaultImage: "lts",
});

const images = multipass.getImagesOutput({ alias: "lts" }, { provider });

const vm = new multipass.Instance("dev", {
    name: instanceName,
    cpus: 2,
    memory: "2G",
    disk: "10G",
    resizePolicy: "in_place",
    cloudInit: "#cloud-config\nwrite_files:\n  - path: /etc/pulumi-example\n    content: provisioned with Pulumi\n",
    waitForCloudInit: true,
    mounts: hostPath ? [{ hostPath, instancePath: "/workspace" }] : [],
    networks: network ? [{ name: network }] : [],
    timeouts: { create: "20m", update: "15m", delete: "10m" },
}, {
    provider,
    // A replacement cannot coexist with a VM using the same physical name.
    deleteBeforeReplace: true,
    customTimeouts: { create: "30m", update: "20m", delete: "15m" },
});

new multipass.Alias("shell", {
    name: `${instanceName}-shell`,
    instance: vm.name,
    command: "bash",
}, { provider, deleteBeforeReplace: true });

// The output reference orders the upload after creation and cloud-init.
const greeting = new multipass.FileUpload("greeting", {
    instance: vm.name,
    destination: "/home/ubuntu/pulumi.txt",
    content: pulumi.interpolate`Hello from ${vm.name}\nIPv4: ${vm.ipv4s.apply(addresses => addresses.join(", "))}\n`,
}, { provider, deleteBeforeReplace: true });

export const name = vm.name;
// The bridge pluralizes Terraform's ipv4 attribute.
export const ipv4 = vm.ipv4s;
export const uploadHash = greeting.contentHash;
export const availableImages = images.images;
