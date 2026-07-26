#!/usr/bin/env ruby
# frozen_string_literal: true

# Adds the TurboistWidget lock-screen widget extension target to the Capacitor
# iOS project. Idempotent: safe to re-run, and it must be re-run if the iOS
# project is ever regenerated from scratch (`yarn cap add ios`), because Capacitor
# scaffolds only the App target.
#
#   gem install xcodeproj    # once
#   ruby frontend/ios/setup-widget.rb
#
# The widget sources live in App/TurboistWidget/ and are checked in alongside the
# project; this script only wires them into App.xcodeproj (a new app-extension
# target, the "Embed App Extensions" phase on App, and the build settings).

require 'xcodeproj'

WIDGET_NAME = 'TurboistWidget'
BUNDLE_ID = 'ru.tinyops.turboist.TurboistWidget'
DEPLOYMENT = '16.0' # lock-screen accessory widgets require iOS 16+

project_path = File.expand_path('App/App.xcodeproj', __dir__)
project = Xcodeproj::Project.open(project_path)

if project.targets.any? { |t| t.name == WIDGET_NAME }
  puts "Target #{WIDGET_NAME} already exists — nothing to do."
  exit 0
end

app_target = project.targets.find { |t| t.name == 'App' }
abort 'App target not found' unless app_target

widget = project.new_target(:app_extension, WIDGET_NAME, :ios, DEPLOYMENT, project.products_group, :swift)

widget.build_configurations.each do |config|
  bs = config.build_settings
  bs['PRODUCT_BUNDLE_IDENTIFIER'] = BUNDLE_ID
  bs['INFOPLIST_FILE'] = "#{WIDGET_NAME}/Info.plist"
  bs['GENERATE_INFOPLIST_FILE'] = 'NO'
  bs['CODE_SIGN_STYLE'] = 'Automatic'
  bs['SWIFT_VERSION'] = '5.0'
  bs['IPHONEOS_DEPLOYMENT_TARGET'] = DEPLOYMENT
  bs['TARGETED_DEVICE_FAMILY'] = '1,2'
  bs['CURRENT_PROJECT_VERSION'] = '1'
  bs['MARKETING_VERSION'] = '1.0'
  bs['SKIP_INSTALL'] = 'YES'
  bs['PRODUCT_NAME'] = '$(TARGET_NAME)'
  bs['SWIFT_EMIT_LOC_STRINGS'] = 'YES'
  bs['LD_RUNPATH_SEARCH_PATHS'] = [
    '$(inherited)',
    '@executable_path/Frameworks',
    '@executable_path/../../Frameworks'
  ]
end

# Source + Info.plist references, grouped under a TurboistWidget folder.
group = project.main_group.new_group(WIDGET_NAME, WIDGET_NAME)
swift_ref = group.new_file('TurboistWidget.swift')
group.new_file('Info.plist')
widget.add_file_references([swift_ref])

widget.add_system_framework(%w[WidgetKit SwiftUI])

# Embed the .appex into the App bundle and build it before App.
app_target.add_dependency(widget)

embed_phase = app_target.copy_files_build_phases.find { |p| p.name == 'Embed App Extensions' }
unless embed_phase
  embed_phase = app_target.new_copy_files_build_phase('Embed App Extensions')
  embed_phase.symbol_dst_subfolder_spec = :plug_ins
end
build_file = embed_phase.add_file_reference(widget.product_reference, true)
build_file.settings = { 'ATTRIBUTES' => ['RemoveHeadersOnCopy'] }

project.save
puts "Added #{WIDGET_NAME} target and embedded it into App."
