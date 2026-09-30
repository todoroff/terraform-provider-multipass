import pulumi
import pulumi_multipass as multipass

config = pulumi.Config()
instance_name = config.get("instanceName") or f"pulumi-py-{pulumi.get_stack()}"
host_path = config.get("hostPath")
network = config.get("network")

provider = multipass.Provider(
    "local",
    multipass_path=config.get("multipassPath"),
    command_timeout=1200,
    default_image="lts",
)

images = multipass.get_images_output(alias="lts", opts=pulumi.InvokeOptions(provider=provider))

vm = multipass.Instance(
    "dev",
    name=instance_name,
    cpus=2,
    memory="2G",
    disk="10G",
    resize_policy="in_place",
    cloud_init="#cloud-config\nwrite_files:\n  - path: /etc/pulumi-example\n    content: provisioned with Pulumi\n",
    wait_for_cloud_init=True,
    mounts=[multipass.InstanceMountArgs(host_path=host_path, instance_path="/workspace")] if host_path else [],
    networks=[multipass.InstanceNetworkArgs(name=network)] if network else [],
    timeouts=multipass.InstanceTimeoutsArgs(create="20m", update="15m", delete="10m"),
    opts=pulumi.ResourceOptions(
        provider=provider,
        # A replacement cannot coexist with a VM using the same physical name.
        delete_before_replace=True,
        custom_timeouts=pulumi.CustomTimeouts(create="30m", update="20m", delete="15m"),
    ),
)

multipass.Alias(
    "shell",
    name=f"{instance_name}-shell",
    instance=vm.name,
    command="bash",
    opts=pulumi.ResourceOptions(provider=provider, delete_before_replace=True),
)

# The output reference orders the upload after creation and cloud-init.
greeting = multipass.FileUpload(
    "greeting",
    instance=vm.name,
    destination="/home/ubuntu/pulumi.txt",
    content=pulumi.Output.all(vm.name, vm.ipv4s).apply(
        lambda values: f"Hello from {values[0]}\nIPv4: {', '.join(values[1])}\n"
    ),
    opts=pulumi.ResourceOptions(provider=provider, delete_before_replace=True),
)

pulumi.export("name", vm.name)
# The bridge pluralizes Terraform's ipv4 attribute.
pulumi.export("ipv4", vm.ipv4s)
pulumi.export("uploadHash", greeting.content_hash)
pulumi.export("availableImages", images.images)
