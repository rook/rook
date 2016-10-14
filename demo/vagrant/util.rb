def set_coreos_box(config, channel)
  config.vm.box = "coreos-%s" % channel
  config.vm.box_url = "http://%s.release.core-os.net/amd64-usr/current/coreos_production_vagrant.json" % $coreos_channel

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
