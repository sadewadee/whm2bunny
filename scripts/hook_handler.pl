#!/usr/bin/perl
# cPanel hook handler for whm2bunny
# This module forwards WHM/cPanel events to the whm2bunny HTTP server

package Whm2bunnyHook;

use strict;
use warnings;
use Cpanel::Logger;
use LWP::UserAgent;
use JSON::XS;
use Digest::SHA qw(hmac_sha256_hex);

# Configuration
my $logger = Cpanel::Logger->new();
my $WHM2BUNNY_URL = $ENV{'WHM2BUNNY_URL'} // 'http://127.0.0.1:9090/hook';
my $SECRET = $ENV{'WHM2BUNNY_SECRET'} // $ENV{'WHM_HOOK_SECRET'} // '';

# Validate secret is set
unless ($SECRET) {
    $logger->warn("WHM2BUNNY_SECRET not set in environment");
}

# LWP UserAgent for HTTP requests
my $ua = LWP::UserAgent->new();
$ua->agent("whm2bunny-hook/1.0");
$ua->timeout(10);
$ua->max_redirect(2);

# Describe hooks to register
sub describe {
    return [
        {
            category => 'Whostmgr::API::1',
            event    => 'createacct',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_account_create',
            exectype => 'module',
        },
        {
            category => 'Whostmgr::API::1',
            event    => 'removeacct',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_account_delete',
            exectype => 'module',
        },
        {
            category => 'Api2',
            event    => 'AddonDomain::addaddondomain',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_addon_create',
            exectype => 'module',
        },
        {
            category => 'Api2',
            event    => 'AddonDomain::deladdondomain',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_addon_delete',
            exectype => 'module',
        },
        {
            category => 'Api2',
            event    => 'SubDomain::addsubdomain',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_subdomain_create',
            exectype => 'module',
        },
        {
            category => 'Api2',
            event    => 'SubDomain::delsubdomain',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_subdomain_delete',
            exectype => 'module',
        },
        {
            category => 'Whostmgr::API::1',
            event    => 'modifyacct',
            stage    => 'post',
            hook     => 'Whm2bunnyHook::handle_account_modify',
            exectype => 'module',
        },
    ];
}

# Handle new account creation
sub handle_account_create {
    my ($context, $data) = @_;

    my $domain = $data->{'domain'} || return;
    my $user = $data->{'user'} || return;

    send_to_whm2bunny({
        event  => 'account_created',
        domain => $domain,
        user   => $user,
        plan   => $data->{'plan'} || '',
    });
}

# Handle account deletion
sub handle_account_delete {
    my ($context, $data) = @_;

    my $domain = $data->{'domain'} || return;
    my $user = $data->{'user'} || return;

    send_to_whm2bunny({
        event  => 'account_deleted',
        domain => $domain,
        user   => $user,
    });
}

# Handle addon domain creation
sub handle_addon_create {
    my ($context, $data) = @_;

    my $domain = $data->{'newdomain'} || return;
    my $user = $context->{'user'} || return;

    send_to_whm2bunny({
        event  => 'addon_created',
        domain => $domain,
        user   => $user,
        parent_domain => $data->{'domain'} || '',
    });
}

# Handle addon domain deletion
sub handle_addon_delete {
    my ($context, $data) = @_;

    my $domain = $data->{'domain'} || return;
    my $user = $context->{'user'} || return;

    send_to_whm2bunny({
        event  => 'addon_deleted',
        domain => $domain,
        user   => $user,
    });
}

# Handle subdomain creation
sub handle_subdomain_create {
    my ($context, $data) = @_;

    my $subdomain = $data->{'subdomain'} || return;
    my $parent_domain = $data->{'domain'} || return;
    my $user = $context->{'user'} || return;

    # Construct full subdomain
    my $full_domain = "$subdomain.$parent_domain";

    send_to_whm2bunny({
        event        => 'subdomain_created',
        subdomain    => $subdomain,
        parent_domain => $parent_domain,
        full_domain  => $full_domain,
        user         => $user,
    });
}

# Handle subdomain deletion
sub handle_subdomain_delete {
    my ($context, $data) = @_;

    my $subdomain = $data->{'subdomain'} || return;
    my $parent_domain = $data->{'domain'} || return;
    my $user = $context->{'user'} || return;

    # Construct full subdomain
    my $full_domain = "$subdomain.$parent_domain";

    send_to_whm2bunny({
        event        => 'subdomain_deleted',
        subdomain    => $subdomain,
        parent_domain => $parent_domain,
        full_domain  => $full_domain,
        user         => $user,
    });
}

# Handle account modification
sub handle_account_modify {
    my ($context, $data) = @_;

    my $domain = $data->{'domain'} || return;
    my $user = $data->{'user'} || return;

    # Only process if domain changed
    if ($data->{'newdomain'}) {
        send_to_whm2bunny({
            event     => 'account_modified',
            domain    => $domain,
            new_domain => $data->{'newdomain'},
            user      => $user,
        });
    }
}

# Send payload to whm2bunny HTTP server
sub send_to_whm2bunny {
    my ($payload) = @_;

    return unless $SECRET;

    # Use canonical (sorted keys) JSON to match Python hook's sort_keys=True
    my $coder = JSON::XS->new->canonical(1)->utf8(1);
    my $json = $coder->encode($payload);

    # Generate HMAC signature
    my $sig = hmac_sha256_hex($json, $SECRET);

    # Send HTTP POST request
    my $response = eval {
        $ua->post(
            $WHM2BUNNY_URL,
            'Content-Type' => 'application/json',
            'X-Whm2bunny-Signature' => $sig,
            'Content' => $json,
        );
    };

    if ($@ || !$response) {
        $logger->error("whm2bunny hook failed: $@");
        return 0;
    }

    if ($response->is_success) {
        $logger->info("whm2bunny hook sent: " . $payload->{'event'} . " for " . ($payload->{'domain'} // $payload->{'subdomain'} // 'unknown'));
        return 1;
    } else {
        $logger->error("whm2bunny hook failed: " . $response->status_line);
        return 0;
    }
}

1;

__END__

=head1 NAME

Whm2bunnyHook - cPanel/WHM hook handler for whm2bunny

=head1 SYNOPSIS

    # Register hooks
    /usr/local/cpanel/bin/manage_hooks add module Whm2bunnyHook

    # List hooks
    /usr/local/cpanel/bin/manage_hooks list module Whm2bunnyHook

    # Delete hooks
    /usr/local/cpanel/bin/manage_hooks delete module Whm2bunnyHook

=head1 DESCRIPTION

This module registers cPanel/WHM hooks that automatically notify the whm2bunny
service when domains are created, modified, or deleted.

=head1 ENVIRONMENT VARIABLES

=over 4

=item * WHM2BUNNY_URL - URL of the whm2bunny webhook endpoint (default: http://127.0.0.1:9090/hook)

=item * WHM2BUNNY_SECRET - Secret for HMAC signature verification (required)

=back

=head1 EVENTS HANDLED

=over 4

=item * account_created - New cPanel account created

=item * account_deleted - cPanel account terminated

=item * account_modified - cPanel account modified

=item * addon_created - Addon domain added

=item * addon_deleted - Addon domain removed

=item * subdomain_created - Subdomain created

=item * subdomain_deleted - Subdomain removed

=back

=head1 AUTHOR

Mordenhost <https://mordenhost.com>

=cut
