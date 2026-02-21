#!/usr/bin/perl
# WHM/cPanel hook handler for whm2bunny
# This script forwards WHM events to the whm2bunny HTTP server

use strict;
use warnings;
use LWP::UserAgent;
use JSON::XS;

# TODO: Configure whm2bunny endpoint URL
my $WHM2BUNNY_URL = "http://localhost:9090/hook";
# TODO: Configure webhook secret
my $WEBHOOK_SECRET = "your-secret-here";

sub send_webhook {
    my ($data) = @_;

    my $ua = LWP::UserAgent->new();
    $ua->agent("whm2bunny-hook/1.0");

    my $response = $ua->post(
        $WHM2BUNNY_URL,
        Content_Type => 'application/json',
        Content => encode_json($data),
        'X-Webhook-Secret' => $WEBHOOK_SECRET,
    );

    unless ($response->is_success) {
        warn "Failed to send webhook: " . $response->status_line;
        return 0;
    }

    return 1;
}

1;
