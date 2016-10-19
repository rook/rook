def set_coreos_box(config, channel)
  config.vm.box = "coreos-%s" % channel
  config.vm.box_url = "http://%s.release.core-os.net/amd64-usr/current/coreos_production_vagrant.json" % $channel

  ["vmware_fusion", "vmware_workstation"].each do |vmware|
    config.vm.provider vmware do |v, override|
      override.vm.box_url = "http://%s.release.core-os.net/amd64-usr/current/coreos_production_vagrant_vmware_fusion.json" % $coreos_channel
    end
  end
end

# a plugin to set the cloud-config file and substitute variables
class CloudConfigPlugin < Vagrant.plugin('2')
  name 'cloud_config'

  config(:'cloud_config', :provisioner) do
    class Config < Vagrant.plugin('2', :config)
      attr_accessor :template
      attr_accessor :subst

      def validate(machine)
        errors = _detected_errors
        if !template
          errors << I18n.t("vagrant.provisioners.cloud_config.template")
        end
        { "Cloud Config provisioner" => errors }
      end
    end
    Config
  end

  provisioner :cloud_config do
    class Provisioner < Vagrant.plugin('2', :provisioner)
      def provision
        @machine.ui.info("Creating cloud-config file")

        configfile = IO.readlines("#{config.template}")[0..-1].join

        config.subst.each do |key, value|
          if ! configfile[key].nil?
            configfile[key]=value
          end
        end

        File.open("cloud-config.yml", 'w') { |file| file.write(configfile) }

        @machine.communicate.tap do |comm|
          comm.upload(File.expand_path("cloud-config.yml"), "/tmp/vagrantfile-user-data")
          command = "mv /tmp/vagrantfile-user-data /var/lib/coreos-vagrant/"
          comm.execute(command, sudo: config.privileged)
        end
      end
    end
    Provisioner
  end
end

class VagrantPlugins::ProviderVirtualBox::Action::SetName
  alias_method :original_call, :call
  def call(env)
    machine = env[:machine]
    driver = machine.provider.driver
    uuid = driver.instance_eval { @uuid }
    ui = env[:ui]

    controller_name="SATA Controller"

    vm_info = driver.execute("showvminfo", uuid)
    controller_already_exists = vm_info.match("Storage Controller Name.*#{controller_name}")

    if controller_already_exists
      ui.info "already has the #{controller_name} hdd controller, skipping creation/add"
    else
      ui.info "creating #{controller_name} hdd controller"
      driver.execute(
        'storagectl',
        uuid,
        '--name', "#{controller_name}",
        '--add', 'sata',
        '--controller', 'IntelAHCI')
    end

    original_call(env)
  end
end

# Add persistent storage volumes
def attach_volumes(node, num_volumes, volume_size)

    node.vm.provider :virtualbox do |v, override|
      (1..num_volumes).each do |disk|
        diskname = File.join(File.dirname(File.expand_path(__FILE__)), ".virtualbox", "#{node.vm.hostname}-#{disk}.vdi")
        unless File.exist?(diskname)
          v.customize ['createhd', '--filename', diskname, '--size', volume_size * 1024]
        end
        v.customize ['storageattach', :id, '--storagectl', 'SATA Controller', '--port', disk, '--device', 0, '--type', 'hdd', '--medium', diskname]
      end
    end

end

