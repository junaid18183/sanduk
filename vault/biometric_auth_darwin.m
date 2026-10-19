#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>

#include "biometric_auth_darwin.h"

// One context per process. sanduk is a short-lived CLI: the context lives for
// the single command that created it and is never shared across invocations,
// so the reuse window cannot outlive the work it was granted for.
static LAContext *sandukAuthContext = nil;

void *sanduk_auth_context(double reuse_seconds) {
	if (sandukAuthContext == nil) {
		sandukAuthContext = [[LAContext alloc] init];
		if (sandukAuthContext == nil) {
			return NULL;
		}
		[sandukAuthContext setTouchIDAuthenticationAllowableReuseDuration:reuse_seconds];
	}
	return (void *)sandukAuthContext;
}
